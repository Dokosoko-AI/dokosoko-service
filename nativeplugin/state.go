package nativeplugin

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrStateNotFound = errors.New("native plugin state not found")
	ErrStateConflict = errors.New("native plugin state revision conflict")
	ErrStateLimit    = errors.New("native plugin state limit exceeded")
)

type StateValue struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	Revision  int64           `json:"revision"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
}

type PutOptions struct {
	ExpectedRevision int64
	ExpiresAt        *time.Time
}

type State interface {
	Get(context.Context, string) (StateValue, error)
	Put(context.Context, string, json.RawMessage, PutOptions) (StateValue, error)
	Delete(context.Context, string, int64) error
	List(context.Context, string, int) ([]StateValue, error)
	CompareAndSwap(context.Context, string, int64, json.RawMessage, *time.Time) (StateValue, error)
	Transaction(context.Context, func(StateTransaction) error) error
}

type StateTransaction interface {
	Get(string) (StateValue, error)
	Put(string, json.RawMessage, PutOptions) (StateValue, error)
	Delete(string, int64) error
	List(string, int) ([]StateValue, error)
}

// UpgradeStore enumerates anonymous, already-bound scopes. Scope identifiers
// are deliberately not exposed to plugin migration code.
type UpgradeStore interface {
	ForEachScope(context.Context, func(State) error) error
}
