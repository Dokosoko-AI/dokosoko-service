package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

type Vault struct{ key []byte }

type Encrypted struct {
	Ciphertext  []byte
	Nonce       []byte
	Fingerprint string
	KeyVersion  int
}

func New(key []byte) (*Vault, error) {
	if len(key) != 32 {
		return nil, errors.New("secret vault key must be 32 bytes")
	}
	return &Vault{key: append([]byte(nil), key...)}, nil
}

func (v *Vault) Encrypt(plaintext []byte, associatedData string) (Encrypted, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return Encrypted{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Encrypted{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return Encrypted{}, err
	}
	digest := sha256.Sum256(plaintext)
	return Encrypted{Ciphertext: gcm.Seal(nil, nonce, plaintext, []byte(associatedData)), Nonce: nonce, Fingerprint: hex.EncodeToString(digest[:8]), KeyVersion: 1}, nil
}

func (v *Vault) Decrypt(value Encrypted, associatedData string) ([]byte, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(value.Nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid secret nonce")
	}
	return gcm.Open(nil, value.Nonce, value.Ciphertext, []byte(associatedData))
}
