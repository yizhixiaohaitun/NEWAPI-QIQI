package middleware

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContextAuditStoreDropsWhenFullWithoutBlocking(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	store := newContextAuditStore(1, 1, 1<<20, func(*model.ContextRequestLog) error {
		startedOnce.Do(func() { close(started) })
		<-release
		return nil
	})
	record := &model.ContextRequestLog{}
	require.True(t, store.enqueue(record))
	<-started
	require.True(t, store.enqueue(record))

	before := time.Now()
	assert.False(t, store.enqueue(record))
	assert.Less(t, time.Since(before), 100*time.Millisecond)
	assert.Equal(t, uint64(1), store.dropped.Load())

	close(release)
	require.NoError(t, store.shutdown(context.Background()))
	assert.Zero(t, store.queuedBytes)
}

func TestContextAuditStoreEnforcesTotalByteBudget(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	store := newContextAuditStore(8, 1, 1024, func(*model.ContextRequestLog) error {
		close(started)
		<-release
		return nil
	})
	entry := &model.ContextRequestLog{RequestBody: string(make([]byte, 300))}
	require.Less(t, estimateContextAuditLogBytes(entry), uint64(1024))
	require.True(t, store.enqueue(entry))
	<-started

	// The active worker item remains charged, so queued plus in-flight records
	// can never exceed the configured process-local audit budget.
	assert.False(t, store.enqueue(entry))
	assert.Equal(t, uint64(1), store.dropped.Load())

	close(release)
	require.NoError(t, store.shutdown(context.Background()))
	assert.Zero(t, store.queuedBytes)
}

func TestContextAuditStoreReleasesBudgetAfterRecordFailure(t *testing.T) {
	recorded := make(chan struct{}, 1)
	store := newContextAuditStore(1, 1, 1<<20, func(*model.ContextRequestLog) error {
		recorded <- struct{}{}
		return errors.New("database unavailable")
	})
	entry := &model.ContextRequestLog{RequestBody: "request", ResponseBody: "response"}
	require.True(t, store.enqueue(entry))
	<-recorded
	require.Eventually(t, func() bool {
		store.mu.RLock()
		defer store.mu.RUnlock()
		return store.queuedBytes == 0
	}, time.Second, time.Millisecond)

	require.True(t, store.enqueue(entry))
	require.NoError(t, store.shutdown(context.Background()))
	assert.Zero(t, store.queuedBytes)
}

func TestEstimateContextAuditLogBytesIncludesRetainedStrings(t *testing.T) {
	base := estimateContextAuditLogBytes(&model.ContextRequestLog{})
	entry := &model.ContextRequestLog{
		RequestBody:    string(make([]byte, 101)),
		ResponseBody:   string(make([]byte, 103)),
		RequestHeaders: string(make([]byte, 107)),
		Error:          string(make([]byte, 109)),
		Path:           string(make([]byte, 113)),
		UserAgent:      string(make([]byte, 127)),
	}
	assert.Equal(t, base+101+103+107+109+113+127, estimateContextAuditLogBytes(entry))
}

func TestContextAuditStoreConcurrentEnqueueAndShutdown(t *testing.T) {
	store := newContextAuditStore(4, 2, 1<<20, func(*model.ContextRequestLog) error { return nil })
	var producers sync.WaitGroup
	for range 16 {
		producers.Add(1)
		go func() {
			defer producers.Done()
			for range 100 {
				store.enqueue(&model.ContextRequestLog{})
			}
		}()
	}
	require.NoError(t, store.shutdown(context.Background()))
	producers.Wait()
	assert.False(t, store.enqueue(&model.ContextRequestLog{}))
	assert.Zero(t, store.queuedBytes)
}
