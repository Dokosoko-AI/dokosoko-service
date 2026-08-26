package nativeplugin

import (
	"context"
)

// NopInstance is useful for manifest-only plugins and API tests.
type NopInstance struct{}

func (NopInstance) Invoke(context.Context, Invocation) (Result, error) {
	return Result{Structured: map[string]any{}}, nil
}

func (NopInstance) Close(context.Context) error { return nil }
