package nativeplugin

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	pluginIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)
	toolIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)*$`)
	namePattern     = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	configPattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	grantPattern    = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$`)
	versionPattern  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)
)

func EnvironmentName(pluginID, key string) string {
	return "DOKOSOKO_PLUGIN_" + strings.ToUpper(pluginID) + "_" + key
}

func ValidateManifest(manifest Manifest) error {
	if !pluginIDPattern.MatchString(manifest.ID) || len(manifest.ID) > 64 {
		return errors.New("native plugin id must be a canonical lower-case underscore identifier")
	}
	if !versionPattern.MatchString(manifest.Version) {
		return errors.New("native plugin version must be semantic version text")
	}
	if manifest.SDKVersion != SDKVersion {
		return fmt.Errorf("native plugin SDK version %d is incompatible with host SDK version %d", manifest.SDKVersion, SDKVersion)
	}
	if strings.TrimSpace(manifest.Description) == "" || len(manifest.Description) > 500 {
		return errors.New("native plugin description is required and must not exceed 500 characters")
	}
	if len(manifest.Config) > 64 {
		return errors.New("native plugin cannot declare more than 64 configuration keys")
	}
	configKeys := make(map[string]ConfigSpec, len(manifest.Config))
	for _, spec := range manifest.Config {
		if !configPattern.MatchString(spec.Key) {
			return fmt.Errorf("native plugin configuration key %q is invalid", spec.Key)
		}
		if _, exists := configKeys[spec.Key]; exists {
			return fmt.Errorf("native plugin configuration key %q is duplicated", spec.Key)
		}
		switch spec.Type {
		case ConfigString, ConfigSecret, ConfigBoolean, ConfigInteger, ConfigDuration, ConfigURL:
		default:
			return fmt.Errorf("native plugin configuration key %q has unsupported type %q", spec.Key, spec.Type)
		}
		if strings.TrimSpace(spec.Description) == "" || len(spec.Description) > 500 {
			return fmt.Errorf("native plugin configuration key %q requires a bounded description", spec.Key)
		}
		configKeys[spec.Key] = spec
	}
	if len(manifest.Network) > 32 {
		return errors.New("native plugin cannot declare more than 32 network destinations")
	}
	networkClaims := make(map[string]bool, len(manifest.Network))
	for _, claim := range manifest.Network {
		if (claim.Host == "") == (claim.ConfigKey == "") {
			return errors.New("native plugin network claim must set exactly one of host or config_key")
		}
		if claim.Host != "" {
			host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(claim.Host)), ".")
			if host == "" || strings.ContainsAny(host, ":/?#@") || net.ParseIP(host) != nil {
				return fmt.Errorf("native plugin network host claim %q is invalid", claim.Host)
			}
		}
		if claim.ConfigKey != "" {
			spec, ok := configKeys[claim.ConfigKey]
			if !ok || spec.Type != ConfigURL {
				return fmt.Errorf("native plugin network claim references non-URL configuration key %q", claim.ConfigKey)
			}
		}
		claimKey := strings.ToLower(strings.TrimSpace(claim.Host)) + "\x00" + claim.ConfigKey
		if networkClaims[claimKey] {
			return errors.New("native plugin network claims must be unique")
		}
		networkClaims[claimKey] = true
	}
	capabilities := make(map[Capability]bool, len(manifest.Capabilities))
	for _, capability := range manifest.Capabilities {
		switch capability {
		case CapabilityNetwork:
		default:
			return fmt.Errorf("native plugin capability %q is unsupported", capability)
		}
		if capabilities[capability] {
			return fmt.Errorf("native plugin capability %q is duplicated", capability)
		}
		capabilities[capability] = true
	}
	if len(manifest.Network) > 0 && !capabilities[CapabilityNetwork] {
		return errors.New("native plugin with network claims must declare the network capability")
	}
	if capabilities[CapabilityNetwork] && len(manifest.Network) == 0 {
		return errors.New("native plugin network capability requires at least one destination claim")
	}
	if len(manifest.Tools) == 0 || len(manifest.Tools) > 64 {
		return errors.New("native plugin must declare between 1 and 64 tools")
	}
	toolIDs, names := make(map[string]bool, len(manifest.Tools)), make(map[string]bool, len(manifest.Tools))
	for _, tool := range manifest.Tools {
		if err := validateToolSpec(tool); err != nil {
			return fmt.Errorf("native plugin tool %q: %w", tool.ID, err)
		}
		if toolIDs[tool.ID] {
			return fmt.Errorf("native plugin tool id %q is duplicated", tool.ID)
		}
		if names[tool.FullName()] {
			return fmt.Errorf("native plugin tool name %q is duplicated", tool.FullName())
		}
		toolIDs[tool.ID], names[tool.FullName()] = true, true
	}
	return nil
}

func validateToolSpec(tool ToolSpec) error {
	if !toolIDPattern.MatchString(tool.ID) || len(tool.ID) > 128 {
		return errors.New("id must be a canonical dotted lower-case identifier")
	}
	if !namePattern.MatchString(tool.Namespace) || !namePattern.MatchString(tool.Name) {
		return errors.New("namespace and name must be lower-case identifiers")
	}
	if strings.TrimSpace(tool.Description) == "" || len(tool.Description) > 500 {
		return errors.New("description is required and must not exceed 500 characters")
	}
	if err := closedObjectSchema(tool.InputSchema); err != nil {
		return fmt.Errorf("input schema: %w", err)
	}
	if err := closedObjectSchema(tool.OutputSchema); err != nil {
		return fmt.Errorf("output schema: %w", err)
	}
	switch tool.Effect {
	case EffectRead, EffectWrite, EffectDestructive:
	default:
		return errors.New("effect must be read, write, or destructive")
	}
	switch tool.Identity {
	case IdentityNone, IdentityOptional, IdentityActorRequired, IdentityCustomerRequired, IdentityActorAndCustomerRequired, IdentityInstallationRequired:
	default:
		return errors.New("identity requirement is unsupported")
	}
	switch tool.StateScope {
	case StateNone, StatePlugin, StateActor, StateCustomer, StateInstallation:
	default:
		return errors.New("state scope is unsupported")
	}
	if tool.StateScope == StateActor && tool.Identity != IdentityActorRequired && tool.Identity != IdentityActorAndCustomerRequired {
		return errors.New("actor state requires actor identity")
	}
	if tool.StateScope == StateCustomer && tool.Identity != IdentityCustomerRequired && tool.Identity != IdentityActorAndCustomerRequired {
		return errors.New("customer state requires customer identity")
	}
	if tool.StateScope == StateInstallation && tool.Identity != IdentityInstallationRequired {
		return errors.New("installation state requires installation identity")
	}
	if tool.Effect == EffectDestructive && !tool.ConfirmationRequired {
		return errors.New("destructive tools require confirmation")
	}
	if tool.Effect != EffectRead && tool.Idempotency != IdempotencyRequired {
		return errors.New("write and destructive tools require idempotency")
	}
	switch tool.Idempotency {
	case IdempotencyNone, IdempotencySupported, IdempotencyRequired:
	default:
		return errors.New("idempotency declaration is unsupported")
	}
	if tool.Timeout < 100*time.Millisecond || tool.Timeout > 60*time.Second {
		return errors.New("timeout must be between 100ms and 60s")
	}
	if tool.MaxConcurrency < 1 || tool.MaxConcurrency > 64 {
		return errors.New("max concurrency must be between 1 and 64")
	}
	if tool.MaxResultBytes < 1 || tool.MaxResultBytes > 1<<20 {
		return errors.New("max result bytes must be between 1 byte and 1 MiB")
	}
	seen := make(map[string]bool, len(tool.RequiredGrants))
	if len(tool.RequiredGrants) > 64 {
		return errors.New("tool cannot require more than 64 grants")
	}
	for _, grant := range tool.RequiredGrants {
		if !grantPattern.MatchString(grant) || seen[grant] {
			return fmt.Errorf("required grant %q is invalid or duplicated", grant)
		}
		seen[grant] = true
	}
	return nil
}

func closedObjectSchema(raw json.RawMessage) error {
	if len(raw) == 0 || len(raw) > 256<<10 {
		return errors.New("schema is missing or exceeds 256 KiB")
	}
	var schema map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&schema); err != nil {
		return err
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		return errors.New("schema root must be a closed object")
	}
	return nil
}

func ManifestHash(manifest Manifest) (string, error) {
	normalized := manifest
	normalized.Config = append([]ConfigSpec(nil), manifest.Config...)
	normalized.Network = append([]NetworkClaim(nil), manifest.Network...)
	normalized.Capabilities = append([]Capability(nil), manifest.Capabilities...)
	normalized.Tools = append([]ToolSpec(nil), manifest.Tools...)
	sort.Slice(normalized.Config, func(i, j int) bool { return normalized.Config[i].Key < normalized.Config[j].Key })
	sort.Slice(normalized.Network, func(i, j int) bool {
		return normalized.Network[i].Host+normalized.Network[i].ConfigKey < normalized.Network[j].Host+normalized.Network[j].ConfigKey
	})
	sort.Slice(normalized.Capabilities, func(i, j int) bool { return normalized.Capabilities[i] < normalized.Capabilities[j] })
	sort.Slice(normalized.Tools, func(i, j int) bool { return normalized.Tools[i].ID < normalized.Tools[j].ID })
	for index := range normalized.Tools {
		normalized.Tools[index].RequiredGrants = append([]string(nil), normalized.Tools[index].RequiredGrants...)
		sort.Strings(normalized.Tools[index].RequiredGrants)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func ToolContractHash(tool ToolSpec) (string, error) {
	normalized := tool
	normalized.RequiredGrants = append([]string(nil), tool.RequiredGrants...)
	sort.Strings(normalized.RequiredGrants)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
