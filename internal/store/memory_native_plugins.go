package store

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/nativepluginstate"
	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

type nativePluginStateScope struct {
	PluginID string
	Kind     string
	ID       string
}

type memoryNativeStateTx struct {
	store  *Memory
	scope  nativePluginStateScope
	now    time.Time
	plugin bool
}

func (m *Memory) StateTransaction(ctx context.Context, pluginID string, scope nativepluginstate.Scope, fn func(nativepluginstate.Transaction) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	scopeKey := nativePluginStateScope{PluginID: pluginID, Kind: scope.Kind, ID: scope.ID}
	prior, existed := cloneNativeStateValues(m.nativePluginState[scopeKey]), m.nativePluginState[scopeKey] != nil
	committed := false
	defer func() {
		if committed {
			return
		}
		if existed {
			m.nativePluginState[scopeKey] = prior
		} else {
			delete(m.nativePluginState, scopeKey)
		}
	}()
	if err := fn(&memoryNativeStateTx{store: m, scope: scopeKey, now: time.Now().UTC()}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (m *Memory) PluginStateTransaction(ctx context.Context, pluginID string, fn func(nativepluginstate.PluginTransaction) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	prior := make(map[nativePluginStateScope]map[string]nativeplugin.StateValue)
	for scope, values := range m.nativePluginState {
		if scope.PluginID == pluginID {
			prior[scope] = cloneNativeStateValues(values)
		}
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		for scope := range m.nativePluginState {
			if scope.PluginID == pluginID {
				delete(m.nativePluginState, scope)
			}
		}
		for scope, values := range prior {
			m.nativePluginState[scope] = values
		}
	}()
	if err := fn(&memoryNativePluginTx{memoryNativeStateTx: memoryNativeStateTx{store: m, scope: nativePluginStateScope{PluginID: pluginID, Kind: string(nativeplugin.StatePlugin)}, now: time.Now().UTC(), plugin: true}, pluginID: pluginID}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	committed = true
	return nil
}

func (tx *memoryNativeStateTx) values() map[string]nativeplugin.StateValue {
	values := tx.store.nativePluginState[tx.scope]
	if values == nil {
		values = make(map[string]nativeplugin.StateValue)
		tx.store.nativePluginState[tx.scope] = values
	}
	return values
}

func (tx *memoryNativeStateTx) Get(key string) (nativeplugin.StateValue, error) {
	value, ok := tx.values()[key]
	if !ok || value.ExpiresAt != nil && !value.ExpiresAt.After(tx.now) {
		if ok {
			delete(tx.values(), key)
		}
		return nativeplugin.StateValue{}, nativeplugin.ErrStateNotFound
	}
	value.Value = append(json.RawMessage(nil), value.Value...)
	return value, nil
}

func (tx *memoryNativeStateTx) Put(key string, raw json.RawMessage, options nativeplugin.PutOptions) (nativeplugin.StateValue, error) {
	values := tx.values()
	current, exists := values[key]
	if exists && current.ExpiresAt != nil && !current.ExpiresAt.After(tx.now) {
		delete(values, key)
		current, exists = nativeplugin.StateValue{}, false
	}
	if options.ExpectedRevision == 0 && exists || options.ExpectedRevision > 0 && (!exists || current.Revision != options.ExpectedRevision) {
		return nativeplugin.StateValue{}, nativeplugin.ErrStateConflict
	}
	if !exists && !strings.HasPrefix(key, "__dokosoko/") {
		count := 0
		for candidate, value := range values {
			if !strings.HasPrefix(candidate, "__dokosoko/") && (value.ExpiresAt == nil || value.ExpiresAt.After(tx.now)) {
				count++
			}
		}
		if count >= nativepluginstate.MaxScopeRecords {
			return nativeplugin.StateValue{}, nativeplugin.ErrStateLimit
		}
	}
	revision := int64(1)
	if exists {
		revision = current.Revision + 1
	}
	value := nativeplugin.StateValue{Key: key, Value: append(json.RawMessage(nil), raw...), Revision: revision, ExpiresAt: cloneTime(options.ExpiresAt)}
	values[key] = value
	return value, nil
}

func (tx *memoryNativeStateTx) Delete(key string, expected int64) error {
	values := tx.values()
	current, exists := values[key]
	if !exists || current.ExpiresAt != nil && !current.ExpiresAt.After(tx.now) {
		return nativeplugin.ErrStateNotFound
	}
	if expected > 0 && current.Revision != expected {
		return nativeplugin.ErrStateConflict
	}
	delete(values, key)
	return nil
}

func (tx *memoryNativeStateTx) List(prefix string, limit int) ([]nativeplugin.StateValue, error) {
	values := tx.values()
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if strings.HasPrefix(key, "__dokosoko/") || !strings.HasPrefix(key, prefix) {
			continue
		}
		if value.ExpiresAt != nil && !value.ExpiresAt.After(tx.now) {
			delete(values, key)
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	result := make([]nativeplugin.StateValue, 0, len(keys))
	for _, key := range keys {
		value := values[key]
		value.Value = append(json.RawMessage(nil), value.Value...)
		result = append(result, value)
	}
	return result, nil
}

type memoryNativePluginTx struct {
	memoryNativeStateTx
	pluginID string
}

func (tx *memoryNativePluginTx) Scopes() []nativepluginstate.Scope {
	seen := map[nativepluginstate.Scope]bool{{Kind: string(nativeplugin.StatePlugin)}: true}
	for scope, values := range tx.store.nativePluginState {
		if scope.PluginID != tx.pluginID {
			continue
		}
		for key, value := range values {
			if strings.HasPrefix(key, "__dokosoko/") || value.ExpiresAt != nil && !value.ExpiresAt.After(tx.now) {
				continue
			}
			seen[nativepluginstate.Scope{Kind: scope.Kind, ID: scope.ID}] = true
			break
		}
	}
	result := make([]nativepluginstate.Scope, 0, len(seen))
	for scope := range seen {
		result = append(result, scope)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Kind+"\x00"+result[i].ID < result[j].Kind+"\x00"+result[j].ID })
	return result
}

func (tx *memoryNativePluginTx) ForScope(scope nativepluginstate.Scope) nativepluginstate.Transaction {
	return &memoryNativeStateTx{store: tx.store, scope: nativePluginStateScope{PluginID: tx.pluginID, Kind: scope.Kind, ID: scope.ID}, now: tx.now}
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneNativeStateValues(values map[string]nativeplugin.StateValue) map[string]nativeplugin.StateValue {
	if values == nil {
		return nil
	}
	result := make(map[string]nativeplugin.StateValue, len(values))
	for key, value := range values {
		value.Value = append(json.RawMessage(nil), value.Value...)
		value.ExpiresAt = cloneTime(value.ExpiresAt)
		result[key] = value
	}
	return result
}

var _ nativepluginstate.Backend = (*Memory)(nil)
