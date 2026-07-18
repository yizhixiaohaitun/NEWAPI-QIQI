package channel_purity

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

const DetectionHeader = "X-New-API-Purity-Detection"

type PairRole string

const (
	PairRoleBaseline PairRole = "baseline"
	PairRoleTarget   PairRole = "target"
)

type PairSnapshot struct {
	ID          string
	Group       string
	ModelFamily string
	RequestType string
	Body        []byte
	ExpiresAt   time.Time
}

type encryptedPair struct {
	group, modelFamily, requestType string
	nonce, ciphertext               []byte
	expiresAt                       time.Time
	consumed                        map[PairRole]bool
}

type PairStore struct {
	mu      sync.Mutex
	aead    cipher.AEAD
	ttl     time.Duration
	entries map[string]*encryptedPair
	now     func() time.Time
}

func NewPairStore(ttl time.Duration) (*PairStore, error) {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &PairStore{aead: aead, ttl: ttl, entries: map[string]*encryptedPair{}, now: time.Now}, nil
}

func (s *PairStore) Put(group, modelFamily, requestType string, body []byte) (string, error) {
	if s == nil || len(body) == 0 {
		return "", errors.New("empty pair snapshot")
	}
	idBytes := make([]byte, 16)
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(idBytes); err != nil {
		return "", err
	}
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	id := hex.EncodeToString(idBytes)
	ciphertext := s.aead.Seal(nil, nonce, body, []byte(id))
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked()
	s.entries[id] = &encryptedPair{group: group, modelFamily: modelFamily, requestType: requestType, nonce: nonce, ciphertext: ciphertext, expiresAt: s.now().Add(s.ttl), consumed: map[PairRole]bool{}}
	return id, nil
}

func (s *PairStore) Consume(id string, role PairRole) (PairSnapshot, error) {
	if role != PairRoleBaseline && role != PairRoleTarget {
		return PairSnapshot{}, errors.New("invalid pair role")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeExpiredLocked()
	entry, ok := s.entries[id]
	if !ok {
		return PairSnapshot{}, errors.New("pair snapshot unavailable")
	}
	if entry.consumed[role] {
		return PairSnapshot{}, errors.New("pair role already consumed")
	}
	body, err := s.aead.Open(nil, entry.nonce, entry.ciphertext, []byte(id))
	if err != nil {
		delete(s.entries, id)
		return PairSnapshot{}, err
	}
	entry.consumed[role] = true
	result := PairSnapshot{ID: id, Group: entry.group, ModelFamily: entry.modelFamily, RequestType: entry.requestType, Body: body, ExpiresAt: entry.expiresAt}
	if entry.consumed[PairRoleBaseline] && entry.consumed[PairRoleTarget] {
		zero(entry.ciphertext)
		delete(s.entries, id)
	}
	return result, nil
}

func (s *PairStore) Destroy(id string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if entry := s.entries[id]; entry != nil {
		zero(entry.ciphertext)
	}
	delete(s.entries, id)
	s.mu.Unlock()
}

func (s *PairStore) purgeExpiredLocked() {
	now := s.now()
	for id, entry := range s.entries {
		if !now.Before(entry.expiresAt) {
			zero(entry.ciphertext)
			delete(s.entries, id)
		}
	}
}

func zero(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
