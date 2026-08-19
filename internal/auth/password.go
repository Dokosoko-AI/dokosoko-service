package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory  = 64 * 1024
	argonTime    = 3
	argonThreads = 2
	argonKeyLen  = 32
)

func HashPassword(password string) (string, error) {
	if !strongPassword(password) {
		return "", ErrPasswordRequirement
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory uint64
	var iterations uint64
	var threads uint64
	for _, pair := range strings.Split(parts[3], ",") {
		values := strings.SplitN(pair, "=", 2)
		if len(values) != 2 {
			return false
		}
		parsed, err := strconv.ParseUint(values[1], 10, 32)
		if err != nil {
			return false
		}
		switch values[0] {
		case "m":
			memory = parsed
		case "t":
			iterations = parsed
		case "p":
			threads = parsed
		}
	}
	if memory < 8*1024 || memory > 256*1024 || iterations < 1 || iterations > 10 || threads < 1 || threads > 8 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, uint32(iterations), uint32(memory), uint8(threads), uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func strongPassword(password string) bool {
	if len(password) < 14 || len(password) > 256 {
		return false
	}
	var lower, upper, digit bool
	for _, value := range password {
		switch {
		case unicode.IsLower(value):
			lower = true
		case unicode.IsUpper(value):
			upper = true
		case unicode.IsDigit(value):
			digit = true
		}
	}
	return lower && upper && digit
}
