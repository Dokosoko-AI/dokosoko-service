package store

import (
	"context"
	"encoding/hex"
	"github.com/dokosoko/dokosoko-service/internal/auth"
	"sort"
	"strings"
	"time"
)

func (m *Memory) SetupCompleted(_ context.Context) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.setupDone, nil
}

func (m *Memory) CreateInitialRoot(_ context.Context, account auth.RootAccount) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.setupDone || len(m.roots) != 0 {
		return ErrConflict
	}
	account.Email = strings.ToLower(account.Email)
	m.roots[account.UserID] = cloneRoot(account)
	m.rootEmail[account.Email] = account.UserID
	m.setupDone = true
	return nil
}

func (m *Memory) CreateRoot(_ context.Context, account auth.RootAccount) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	account.Email = strings.ToLower(account.Email)
	if _, exists := m.rootEmail[account.Email]; exists {
		return ErrConflict
	}
	m.roots[account.UserID] = cloneRoot(account)
	m.rootEmail[account.Email] = account.UserID
	return nil
}

func (m *Memory) RevokeRoot(_ context.Context, userID string, revokedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	account, ok := m.roots[userID]
	if !ok {
		return ErrNotFound
	}
	active := 0
	for _, value := range m.roots {
		if value.RevokedAt == nil {
			active++
		}
	}
	if account.RevokedAt == nil && active <= 1 {
		return auth.ErrLastRoot
	}
	account.RevokedAt = &revokedAt
	m.roots[userID] = account
	for key, session := range m.sessions {
		if session.UserID == userID {
			delete(m.sessions, key)
		}
	}
	return nil
}

func (m *Memory) RootByEmail(_ context.Context, email string) (auth.RootAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.rootEmail[strings.ToLower(email)]
	if !ok {
		return auth.RootAccount{}, ErrNotFound
	}
	return cloneRoot(m.roots[id]), nil
}

func (m *Memory) RootByID(_ context.Context, id string) (auth.RootAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	account, ok := m.roots[id]
	if !ok {
		return auth.RootAccount{}, ErrNotFound
	}
	return cloneRoot(account), nil
}

func (m *Memory) RootAccounts(_ context.Context) ([]auth.RootAccount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]auth.RootAccount, 0, len(m.roots))
	for _, account := range m.roots {
		result = append(result, cloneRoot(account))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func (m *Memory) CreateSession(_ context.Context, session auth.SessionRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.roots[session.UserID]; !ok {
		return ErrNotFound
	}
	m.sessions[hex.EncodeToString(session.TokenDigest)] = cloneSession(session)
	return nil
}

func (m *Memory) SessionByDigest(_ context.Context, digest []byte) (auth.SessionRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[hex.EncodeToString(digest)]
	if !ok {
		return auth.SessionRecord{}, ErrNotFound
	}
	return cloneSession(session), nil
}

func (m *Memory) DeleteSession(_ context.Context, digest []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, hex.EncodeToString(digest))
	return nil
}
