package nativeplugin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validTestManifest() Manifest {
	return Manifest{
		ID: "validation_test", Version: "1.0.0", SDKVersion: SDKVersion,
		Description: "Validate the public native plugin source contract.",
		Tools: []ToolSpec{{
			ID: "status", Namespace: "validation", Name: "status", Description: "Return validation status.",
			InputSchema:  json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{}}`),
			OutputSchema: json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"ok":{"type":"boolean"}},"required":["ok"]}`),
			Effect:       EffectRead, Identity: IdentityNone, StateScope: StateNone,
			Idempotency: IdempotencySupported, Timeout: time.Second, MaxConcurrency: 1, MaxResultBytes: 4096,
		}},
	}
}

func TestValidateManifestRequiresToolsAndLeastPrivilegeNetworkClaims(t *testing.T) {
	valid := validTestManifest()
	if err := ValidateManifest(valid); err != nil {
		t.Fatal(err)
	}
	withoutTools := valid
	withoutTools.Tools = nil
	if err := ValidateManifest(withoutTools); err == nil || !strings.Contains(err.Error(), "between 1 and 64 tools") {
		t.Fatalf("missing tools error = %v", err)
	}
	capabilityOnly := valid
	capabilityOnly.Capabilities = []Capability{CapabilityNetwork}
	if err := ValidateManifest(capabilityOnly); err == nil || !strings.Contains(err.Error(), "requires at least one") {
		t.Fatalf("empty network capability error = %v", err)
	}
}

func TestValidateManifestRequiresIdentityForIdentityScopedState(t *testing.T) {
	manifest := validTestManifest()
	manifest.Tools[0].Identity = IdentityOptional
	manifest.Tools[0].StateScope = StateActor
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "actor state requires actor identity") {
		t.Fatalf("identity-scope error = %v", err)
	}
}
