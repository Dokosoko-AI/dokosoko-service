package nativepluginstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

const (
	MaxKeyBytes     = 128
	MaxValueBytes   = 64 << 10
	MaxListItems    = 100
	MaxScopeRecords = 1000
	reservedPrefix  = "__dokosoko/"
)

const (
	StateVersionKey = reservedPrefix + "state_version"
	EnabledKey      = reservedPrefix + "enabled"
)

type Scope struct {
	Kind string
	ID   string
}

type Transaction interface {
	Get(string) (nativeplugin.StateValue, error)
	Put(string, json.RawMessage, nativeplugin.PutOptions) (nativeplugin.StateValue, error)
	Delete(string, int64) error
	List(string, int) ([]nativeplugin.StateValue, error)
}

type PluginTransaction interface {
	Transaction
	Scopes() []Scope
	ForScope(Scope) Transaction
}

type Backend interface {
	StateTransaction(context.Context, string, Scope, func(Transaction) error) error
	PluginStateTransaction(context.Context, string, func(PluginTransaction) error) error
}

type Bound struct {
	backend  Backend
	pluginID string
	scope    Scope
}

func Bind(backend Backend, pluginID, scopeKind, scopeID string) nativeplugin.State {
	if backend == nil || scopeKind == string(nativeplugin.StateNone) {
		return disabledState{}
	}
	return &Bound{backend: backend, pluginID: pluginID, scope: Scope{Kind: scopeKind, ID: scopeID}}
}

func (s *Bound) Get(ctx context.Context, key string) (nativeplugin.StateValue, error) {
	if err := validateKey(key); err != nil {
		return nativeplugin.StateValue{}, err
	}
	var value nativeplugin.StateValue
	err := s.backend.StateTransaction(ctx, s.pluginID, s.scope, func(tx Transaction) error {
		var err error
		value, err = tx.Get(key)
		return err
	})
	return value, err
}

func (s *Bound) Put(ctx context.Context, key string, value json.RawMessage, options nativeplugin.PutOptions) (nativeplugin.StateValue, error) {
	if err := validateWrite(key, value, options); err != nil {
		return nativeplugin.StateValue{}, err
	}
	var stored nativeplugin.StateValue
	err := s.backend.StateTransaction(ctx, s.pluginID, s.scope, func(tx Transaction) error {
		var err error
		stored, err = tx.Put(key, value, options)
		return err
	})
	return stored, err
}

func (s *Bound) Delete(ctx context.Context, key string, expected int64) error {
	if err := validateKey(key); err != nil {
		return err
	}
	return s.backend.StateTransaction(ctx, s.pluginID, s.scope, func(tx Transaction) error { return tx.Delete(key, expected) })
}

func (s *Bound) List(ctx context.Context, prefix string, limit int) ([]nativeplugin.StateValue, error) {
	if err := validatePrefix(prefix); err != nil {
		return nil, err
	}
	limit = boundedLimit(limit)
	var values []nativeplugin.StateValue
	err := s.backend.StateTransaction(ctx, s.pluginID, s.scope, func(tx Transaction) error {
		var err error
		values, err = tx.List(prefix, limit)
		return err
	})
	return values, err
}

func (s *Bound) CompareAndSwap(ctx context.Context, key string, expected int64, value json.RawMessage, expiresAt *time.Time) (nativeplugin.StateValue, error) {
	return s.Put(ctx, key, value, nativeplugin.PutOptions{ExpectedRevision: expected, ExpiresAt: expiresAt})
}

func (s *Bound) Transaction(ctx context.Context, fn func(nativeplugin.StateTransaction) error) error {
	if fn == nil {
		return errors.New("native plugin state transaction callback is required")
	}
	return s.backend.StateTransaction(ctx, s.pluginID, s.scope, func(tx Transaction) error {
		return fn(guardedTransaction{tx: tx})
	})
}

type guardedTransaction struct{ tx Transaction }

func (g guardedTransaction) Get(key string) (nativeplugin.StateValue, error) {
	if err := validateKey(key); err != nil {
		return nativeplugin.StateValue{}, err
	}
	return g.tx.Get(key)
}

func (g guardedTransaction) Put(key string, value json.RawMessage, options nativeplugin.PutOptions) (nativeplugin.StateValue, error) {
	if err := validateWrite(key, value, options); err != nil {
		return nativeplugin.StateValue{}, err
	}
	return g.tx.Put(key, value, options)
}

func (g guardedTransaction) Delete(key string, expected int64) error {
	if err := validateKey(key); err != nil {
		return err
	}
	return g.tx.Delete(key, expected)
}

func (g guardedTransaction) List(prefix string, limit int) ([]nativeplugin.StateValue, error) {
	if err := validatePrefix(prefix); err != nil {
		return nil, err
	}
	return g.tx.List(prefix, boundedLimit(limit))
}

type upgradeStore struct{ tx PluginTransaction }

func (u upgradeStore) ForEachScope(ctx context.Context, fn func(nativeplugin.State) error) error {
	if fn == nil {
		return errors.New("native plugin upgrade scope callback is required")
	}
	for _, scope := range u.tx.Scopes() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(transactionState{tx: u.tx.ForScope(scope)}); err != nil {
			return err
		}
	}
	return nil
}

type transactionState struct{ tx Transaction }

func (s transactionState) Get(_ context.Context, key string) (nativeplugin.StateValue, error) {
	return guardedTransaction{s.tx}.Get(key)
}
func (s transactionState) Put(_ context.Context, key string, value json.RawMessage, options nativeplugin.PutOptions) (nativeplugin.StateValue, error) {
	return guardedTransaction{s.tx}.Put(key, value, options)
}
func (s transactionState) Delete(_ context.Context, key string, expected int64) error {
	return guardedTransaction{s.tx}.Delete(key, expected)
}
func (s transactionState) List(_ context.Context, prefix string, limit int) ([]nativeplugin.StateValue, error) {
	return guardedTransaction{s.tx}.List(prefix, limit)
}
func (s transactionState) CompareAndSwap(ctx context.Context, key string, expected int64, value json.RawMessage, expiresAt *time.Time) (nativeplugin.StateValue, error) {
	return s.Put(ctx, key, value, nativeplugin.PutOptions{ExpectedRevision: expected, ExpiresAt: expiresAt})
}
func (s transactionState) Transaction(_ context.Context, fn func(nativeplugin.StateTransaction) error) error {
	return fn(guardedTransaction{s.tx})
}

func CurrentVersion(ctx context.Context, backend Backend, pluginID string) (uint32, error) {
	var version uint32
	err := backend.PluginStateTransaction(ctx, pluginID, func(tx PluginTransaction) error {
		value, err := tx.Get(StateVersionKey)
		if errors.Is(err, nativeplugin.ErrStateNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return json.Unmarshal(value.Value, &version)
	})
	return version, err
}

func Upgrade(ctx context.Context, backend Backend, pluginID string, from, to uint32, fn func(nativeplugin.UpgradeStore) error) error {
	if to < from {
		return errors.New("native plugin state downgrade is not allowed")
	}
	return backend.PluginStateTransaction(ctx, pluginID, func(tx PluginTransaction) error {
		current := uint32(0)
		if value, err := tx.Get(StateVersionKey); err == nil {
			if err := json.Unmarshal(value.Value, &current); err != nil {
				return errors.New("native plugin stored state version is invalid")
			}
		} else if !errors.Is(err, nativeplugin.ErrStateNotFound) {
			return err
		}
		if current != from {
			return nativeplugin.ErrStateConflict
		}
		if fn != nil && to != from {
			if err := fn(upgradeStore{tx: tx}); err != nil {
				return err
			}
		}
		encoded, _ := json.Marshal(to)
		_, err := tx.Put(StateVersionKey, encoded, nativeplugin.PutOptions{ExpectedRevision: -1})
		return err
	})
}

func Enabled(ctx context.Context, backend Backend, pluginID string) (bool, error) {
	enabled := true
	err := backend.PluginStateTransaction(ctx, pluginID, func(tx PluginTransaction) error {
		value, err := tx.Get(EnabledKey)
		if errors.Is(err, nativeplugin.ErrStateNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return json.Unmarshal(value.Value, &enabled)
	})
	return enabled, err
}

func SetEnabled(ctx context.Context, backend Backend, pluginID string, enabled bool) error {
	return backend.PluginStateTransaction(ctx, pluginID, func(tx PluginTransaction) error {
		encoded, _ := json.Marshal(enabled)
		_, err := tx.Put(EnabledKey, encoded, nativeplugin.PutOptions{ExpectedRevision: -1})
		return err
	})
}

func validateKey(key string) error {
	if key == "" || len(key) > MaxKeyBytes || strings.HasPrefix(key, reservedPrefix) || strings.ContainsRune(key, 0) {
		return fmt.Errorf("%w: state key is invalid", nativeplugin.ErrStateLimit)
	}
	return nil
}

func validatePrefix(prefix string) error {
	if len(prefix) > MaxKeyBytes || strings.HasPrefix(prefix, reservedPrefix) || strings.ContainsRune(prefix, 0) {
		return fmt.Errorf("%w: state prefix is invalid", nativeplugin.ErrStateLimit)
	}
	return nil
}

func validateWrite(key string, value json.RawMessage, options nativeplugin.PutOptions) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if options.ExpectedRevision < 0 {
		return fmt.Errorf("%w: expected revision must be zero for create or a positive current revision", nativeplugin.ErrStateLimit)
	}
	if len(value) == 0 || len(value) > MaxValueBytes || !json.Valid(value) {
		return fmt.Errorf("%w: state value must be valid JSON no larger than %d bytes", nativeplugin.ErrStateLimit, MaxValueBytes)
	}
	return nil
}

func boundedLimit(limit int) int {
	if limit < 1 {
		return MaxListItems
	}
	if limit > MaxListItems {
		return MaxListItems
	}
	return limit
}

type disabledState struct{}

func (disabledState) Get(context.Context, string) (nativeplugin.StateValue, error) {
	return nativeplugin.StateValue{}, nativeplugin.ErrStateNotFound
}
func (disabledState) Put(context.Context, string, json.RawMessage, nativeplugin.PutOptions) (nativeplugin.StateValue, error) {
	return nativeplugin.StateValue{}, errors.New("native plugin tool did not declare state")
}
func (disabledState) Delete(context.Context, string, int64) error {
	return errors.New("native plugin tool did not declare state")
}
func (disabledState) List(context.Context, string, int) ([]nativeplugin.StateValue, error) {
	return nil, errors.New("native plugin tool did not declare state")
}
func (disabledState) CompareAndSwap(context.Context, string, int64, json.RawMessage, *time.Time) (nativeplugin.StateValue, error) {
	return nativeplugin.StateValue{}, errors.New("native plugin tool did not declare state")
}
func (disabledState) Transaction(context.Context, func(nativeplugin.StateTransaction) error) error {
	return errors.New("native plugin tool did not declare state")
}
