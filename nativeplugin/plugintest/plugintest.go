// Package plugintest provides conformance helpers for native tool plugin authors.
package plugintest

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

// TestPlugin validates the stable, deterministic source contract exposed by a plugin.
func TestPlugin(t *testing.T, plugin nativeplugin.Plugin) {
	t.Helper()
	if plugin == nil || reflect.ValueOf(plugin).Kind() == reflect.Pointer && reflect.ValueOf(plugin).IsNil() {
		t.Fatal("native plugin is nil")
	}
	first := describeSafely(t, plugin)
	second := describeSafely(t, plugin)
	if err := nativeplugin.ValidateManifest(first); err != nil {
		t.Fatalf("manifest is invalid: %v", err)
	}
	left, _ := json.Marshal(first)
	right, _ := json.Marshal(second)
	if string(left) != string(right) {
		t.Fatal("Describe is not deterministic")
	}
	if _, err := nativeplugin.ManifestHash(first); err != nil {
		t.Fatalf("manifest cannot be hashed: %v", err)
	}
}

func describeSafely(t *testing.T, plugin nativeplugin.Plugin) (manifest nativeplugin.Manifest) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Describe panicked: %v", recovered)
		}
	}()
	return plugin.Describe()
}
