package runtimeauth

import (
	"bytes"
	"testing"
)

func TestCredentialBundleRoundTripAndLegacyCompatibility(t *testing.T) {
	legacy := []byte("legacy-secret")
	primary, headers, bundled, err := Decode(legacy)
	if err != nil || bundled || !bytes.Equal(primary, legacy) || len(headers) != 0 {
		t.Fatalf("legacy decode = %q %#v %t %v", primary, headers, bundled, err)
	}

	encoded, err := Encode([]byte("primary-secret"), []Header{{Name: "X-Tenant-Key", Value: []byte("tenant-secret")}})
	if err != nil {
		t.Fatal(err)
	}
	primary, headers, bundled, err = Decode(encoded)
	if err != nil || !bundled || string(primary) != "primary-secret" || len(headers) != 1 || headers[0].Name != "X-Tenant-Key" || string(headers[0].Value) != "tenant-secret" {
		t.Fatalf("bundle decode = %q %#v %t %v", primary, headers, bundled, err)
	}
}

func TestCredentialBundleRejectsUnsafeAndDuplicateHeaders(t *testing.T) {
	for name, headers := range map[string][]Header{
		"authorization": {{Name: "Authorization", Value: []byte("secret")}},
		"duplicate":     {{Name: "X-Key", Value: []byte("one")}, {Name: "x-key", Value: []byte("two")}},
		"newline":       {{Name: "X-Key", Value: []byte("one\ntwo")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Encode([]byte("primary"), headers); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
