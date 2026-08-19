package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

func TOTP(secret []byte, at time.Time) string {
	counter := uint64(at.Unix() / 30)
	message := make([]byte, 8)
	binary.BigEndian.PutUint64(message, counter)
	hash := hmac.New(sha1.New, secret)
	_, _ = hash.Write(message)
	sum := hash.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 | uint32(sum[offset+1])<<16 | uint32(sum[offset+2])<<8 | uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1_000_000)
}

func VerifyTOTP(secret []byte, code string, at time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	result := 0
	for offset := -1; offset <= 1; offset++ {
		expected := TOTP(secret, at.Add(time.Duration(offset)*30*time.Second))
		result |= subtle.ConstantTimeCompare([]byte(expected), []byte(code))
	}
	return result == 1
}
