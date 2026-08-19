package secrets_test

import (
	"bytes"
	"testing"

	"github.com/dokosoko/dokosoko/v2/internal/secrets"
)

func TestVaultEncryptsWithContextBinding(t *testing.T) {
	t.Parallel()
	vault, err := secrets.New(bytes.Repeat([]byte{0x31}, 32))
	if err != nil {
		t.Fatal(err)
	}
	value, err := vault.Encrypt([]byte("vendor-token"), "org-a:package")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(value.Ciphertext, []byte("vendor-token")) || value.Fingerprint == "" {
		t.Fatal("secret was not protected")
	}
	plaintext, err := vault.Decrypt(value, "org-a:package")
	if err != nil || string(plaintext) != "vendor-token" {
		t.Fatalf("decrypt = %q, %v", plaintext, err)
	}
	if _, err := vault.Decrypt(value, "org-b:package"); err == nil {
		t.Fatal("ciphertext decrypted under the wrong tenant context")
	}
}
