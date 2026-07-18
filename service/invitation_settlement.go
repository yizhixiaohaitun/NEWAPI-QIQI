package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	invitationSettlementRetryInterval = time.Minute
	invitationSettlementRetryBatch    = 100
)

var invitationSettlementRetryOnce sync.Once

// StartInvitationSettlementRetryTask resumes durable invitation settlements
// that could not be completed during registration. The model transaction keeps
// this safe even if more than one master scans the same rows.
func StartInvitationSettlementRetryTask() {
	invitationSettlementRetryOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			runInvitationSettlementRetryPass()
			ticker := time.NewTicker(invitationSettlementRetryInterval)
			defer ticker.Stop()
			for range ticker.C {
				runInvitationSettlementRetryPass()
			}
		})
	})
}

func runInvitationSettlementRetryPass() {
	result := model.RetryInvitationSettlements(invitationSettlementRetryBatch)
	if result.Scanned == 0 && result.Failed == 0 {
		return
	}
	message := fmt.Sprintf("invitation settlement retry pass: scanned=%d settled=%d failed=%d", result.Scanned, result.Settled, result.Failed)
	if result.Failed > 0 {
		logger.LogWarn(context.Background(), message)
		return
	}
	logger.LogInfo(context.Background(), message)
}
