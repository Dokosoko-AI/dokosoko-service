package nativepluginstate_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/nativepluginstate"
	"github.com/dokosoko/dokosoko-service/internal/store"
	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

func TestBoundStateIsolationCASAndRollback(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	first := nativepluginstate.Bind(memory, "plugin_one", string(nativeplugin.StateCustomer), "customer_a")
	second := nativepluginstate.Bind(memory, "plugin_one", string(nativeplugin.StateCustomer), "customer_b")

	created, err := first.Put(ctx, "counter", json.RawMessage(`1`), nativeplugin.PutOptions{ExpectedRevision: 0})
	if err != nil || created.Revision != 1 {
		t.Fatalf("create = %#v, %v", created, err)
	}
	if _, err := second.Get(ctx, "counter"); !errors.Is(err, nativeplugin.ErrStateNotFound) {
		t.Fatalf("isolated get error = %v", err)
	}
	if _, err := first.CompareAndSwap(ctx, "counter", 99, json.RawMessage(`2`), nil); !errors.Is(err, nativeplugin.ErrStateConflict) {
		t.Fatalf("stale CAS error = %v", err)
	}
	if _, err := first.Put(ctx, "counter", json.RawMessage(`2`), nativeplugin.PutOptions{ExpectedRevision: -1}); !errors.Is(err, nativeplugin.ErrStateLimit) {
		t.Fatalf("unconditional plugin write error = %v", err)
	}

	rollback := errors.New("rollback")
	err = first.Transaction(ctx, func(tx nativeplugin.StateTransaction) error {
		if _, err := tx.Put("counter", json.RawMessage(`2`), nativeplugin.PutOptions{ExpectedRevision: created.Revision}); err != nil {
			return err
		}
		if _, err := tx.Put("transient", json.RawMessage(`true`), nativeplugin.PutOptions{ExpectedRevision: 0}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("transaction error = %v", err)
	}
	current, err := first.Get(ctx, "counter")
	if err != nil || string(current.Value) != "1" || current.Revision != 1 {
		t.Fatalf("rolled back value = %#v, %v", current, err)
	}
	if _, err := first.Get(ctx, "transient"); !errors.Is(err, nativeplugin.ErrStateNotFound) {
		t.Fatalf("transient value survived rollback: %v", err)
	}
	func() {
		defer func() { _ = recover() }()
		_ = first.Transaction(ctx, func(tx nativeplugin.StateTransaction) error {
			_, _ = tx.Put("panic", json.RawMessage(`true`), nativeplugin.PutOptions{ExpectedRevision: 0})
			panic("plugin callback panic")
		})
	}()
	if _, err := first.Get(ctx, "panic"); !errors.Is(err, nativeplugin.ErrStateNotFound) {
		t.Fatalf("panicked transaction survived rollback: %v", err)
	}
}

func TestStateUpgradeIsAtomicAndMetadataIsPrivate(t *testing.T) {
	ctx := context.Background()
	memory := store.NewMemory()
	state := nativepluginstate.Bind(memory, "plugin_upgrade", string(nativeplugin.StatePlugin), "")
	if _, err := state.Put(ctx, "record", json.RawMessage(`{"version":0}`), nativeplugin.PutOptions{ExpectedRevision: 0}); err != nil {
		t.Fatal(err)
	}
	failed := errors.New("migration failed")
	err := nativepluginstate.Upgrade(ctx, memory, "plugin_upgrade", 0, 1, func(upgrade nativeplugin.UpgradeStore) error {
		return upgrade.ForEachScope(ctx, func(scope nativeplugin.State) error {
			if _, err := scope.Put(ctx, "new", json.RawMessage(`true`), nativeplugin.PutOptions{ExpectedRevision: 0}); err != nil {
				return err
			}
			return failed
		})
	})
	if !errors.Is(err, failed) {
		t.Fatalf("upgrade error = %v", err)
	}
	version, err := nativepluginstate.CurrentVersion(ctx, memory, "plugin_upgrade")
	if err != nil || version != 0 {
		t.Fatalf("version = %d, %v", version, err)
	}
	if _, err := state.Get(ctx, "new"); !errors.Is(err, nativeplugin.ErrStateNotFound) {
		t.Fatalf("failed migration survived: %v", err)
	}
	if _, err := state.Get(ctx, nativepluginstate.StateVersionKey); !errors.Is(err, nativeplugin.ErrStateLimit) {
		t.Fatalf("reserved metadata was exposed: %v", err)
	}
}
