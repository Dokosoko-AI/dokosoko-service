// Package sampleplugin demonstrates a trusted native tool plugin. It is not
// registered by default; clone it and add its constructor to the explicit
// application registry after reviewing the source.
package sampleplugin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

const (
	pluginID  = "example_counter"
	echoTool  = "echo"
	countTool = "counter.increment"
)

type Plugin struct{}

func New() nativeplugin.Plugin { return Plugin{} }

func (Plugin) Describe() nativeplugin.Manifest {
	return nativeplugin.Manifest{
		ID:          pluginID,
		Version:     "1.0.0",
		SDKVersion:  nativeplugin.SDKVersion,
		Description: "Example identity-aware echo and customer-scoped counter tools.",
		Config: []nativeplugin.ConfigSpec{{
			Key: "GREETING_PREFIX", Type: nativeplugin.ConfigString,
			Description: "Optional text placed before echoed messages.",
		}},
		StateVersion: 1,
		Tools: []nativeplugin.ToolSpec{
			{
				ID: echoTool, Namespace: "example", Name: "echo",
				Description:  "Echo a message and note whether an opaque actor identity was available.",
				InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"message":{"type":"string","maxLength":500}},"required":["message"]}`),
				OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"message":{"type":"string"},"identified":{"type":"boolean"}},"required":["message","identified"]}`),
				Effect:       nativeplugin.EffectRead, Identity: nativeplugin.IdentityOptional,
				StateScope: nativeplugin.StateNone, Idempotency: nativeplugin.IdempotencySupported,
				Timeout: 2 * time.Second, MaxConcurrency: 8, MaxResultBytes: 32 << 10,
			},
			{
				ID: countTool, Namespace: "example", Name: "increment_counter",
				Description:  "Increment one counter in the authenticated customer's isolated state scope.",
				InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string","pattern":"^[a-z][a-z0-9_]{0,31}$"}},"required":["name"]}`),
				OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"count":{"type":"integer","minimum":1}},"required":["count"]}`),
				Effect:       nativeplugin.EffectWrite, Identity: nativeplugin.IdentityCustomerRequired,
				StateScope: nativeplugin.StateCustomer, Idempotency: nativeplugin.IdempotencyRequired,
				Timeout: 2 * time.Second, MaxConcurrency: 4, MaxResultBytes: 8 << 10,
			},
		},
	}
}

func (Plugin) Open(_ context.Context, host nativeplugin.Host) (nativeplugin.Instance, error) {
	prefix, _ := host.Config().String("GREETING_PREFIX")
	host.Logger().Info("example plugin opened")
	return &instance{prefix: prefix}, nil
}

type instance struct{ prefix string }

func (i *instance) Invoke(ctx context.Context, call nativeplugin.Invocation) (nativeplugin.Result, error) {
	switch call.ToolID {
	case echoTool:
		message, _ := call.Arguments["message"].(string)
		return nativeplugin.Result{Structured: map[string]any{"message": i.prefix + message, "identified": call.Identity.Actor != nil}}, nil
	case countTool:
		name, _ := call.Arguments["name"].(string)
		if name == "" || call.IdempotencyKey == "" {
			return nativeplugin.Result{}, nativeplugin.Fail(nativeplugin.ErrorInvalidArgument, "A counter name and idempotency key are required", nil)
		}
		count, err := incrementOnce(ctx, call.State, name, call.IdempotencyKey)
		if err != nil {
			return nativeplugin.Result{}, nativeplugin.Fail(nativeplugin.ErrorInternal, "The counter could not be updated", err)
		}
		return nativeplugin.Result{Structured: map[string]any{"count": count}}, nil
	default:
		return nativeplugin.Result{}, nativeplugin.Fail(nativeplugin.ErrorNotFound, "Native tool not found", nil)
	}
}

func incrementOnce(ctx context.Context, state nativeplugin.State, name, idempotencyKey string) (int64, error) {
	var count int64
	idempotencyDigest := sha256.Sum256([]byte(idempotencyKey))
	err := state.Transaction(ctx, func(tx nativeplugin.StateTransaction) error {
		idempotencyStateKey := fmt.Sprintf("idempotency/%x", idempotencyDigest[:])
		if prior, err := tx.Get(idempotencyStateKey); err == nil {
			return json.Unmarshal(prior.Value, &count)
		} else if !errors.Is(err, nativeplugin.ErrStateNotFound) {
			return err
		}
		counterStateKey := "counter/" + name
		prior, err := tx.Get(counterStateKey)
		expectedRevision := int64(0)
		if err == nil {
			expectedRevision = prior.Revision
			if err := json.Unmarshal(prior.Value, &count); err != nil {
				return fmt.Errorf("decode counter: %w", err)
			}
		} else if !errors.Is(err, nativeplugin.ErrStateNotFound) {
			return err
		}
		count++
		encoded, _ := json.Marshal(count)
		if _, err := tx.Put(counterStateKey, encoded, nativeplugin.PutOptions{ExpectedRevision: expectedRevision}); err != nil {
			return err
		}
		_, err = tx.Put(idempotencyStateKey, encoded, nativeplugin.PutOptions{ExpectedRevision: 0})
		return err
	})
	return count, err
}

func (*instance) Close(context.Context) error { return nil }
