package nativeplugin

import (
	"net/url"
	"strconv"
	"time"
)

type Secret struct{ value string }

func NewSecret(value string) Secret { return Secret{value: value} }

func (Secret) String() string { return "[REDACTED]" }

func (Secret) GoString() string { return "[REDACTED]" }

// Reveal returns the configured secret. Callers must not log or persist it.
func (s Secret) Reveal() string { return s.value }

type ConfigValue struct {
	Type   ConfigType
	String string
	Secret Secret
}

type Config struct{ values map[string]ConfigValue }

func NewConfig(values map[string]ConfigValue) Config {
	copyValues := make(map[string]ConfigValue, len(values))
	for key, value := range values {
		copyValues[key] = value
	}
	return Config{values: copyValues}
}

func (c Config) Has(key string) bool { _, ok := c.values[key]; return ok }

func (c Config) String(key string) (string, bool) {
	value, ok := c.values[key]
	if !ok || value.Type == ConfigSecret {
		return "", false
	}
	return value.String, true
}

func (c Config) Secret(key string) (Secret, bool) {
	value, ok := c.values[key]
	return value.Secret, ok && value.Type == ConfigSecret
}

func (c Config) Bool(key string) (bool, bool) {
	value, ok := c.String(key)
	if !ok {
		return false, false
	}
	parsed, err := strconv.ParseBool(value)
	return parsed, err == nil
}

func (c Config) Integer(key string) (int64, bool) {
	value, ok := c.String(key)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil
}

func (c Config) Duration(key string) (time.Duration, bool) {
	value, ok := c.String(key)
	if !ok {
		return 0, false
	}
	parsed, err := time.ParseDuration(value)
	return parsed, err == nil
}

func (c Config) URL(key string) (*url.URL, bool) {
	value, ok := c.String(key)
	if !ok {
		return nil, false
	}
	parsed, err := url.Parse(value)
	return parsed, err == nil && parsed.IsAbs()
}
