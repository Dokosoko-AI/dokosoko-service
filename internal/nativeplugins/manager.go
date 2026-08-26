package nativeplugins

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dokosoko/dokosoko-service/internal/model"
	"github.com/dokosoko/dokosoko-service/internal/nativepluginstate"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

const (
	StatusDiscovered    = "discovered"
	StatusMisconfigured = "misconfigured"
	StatusUpgrading     = "upgrading"
	StatusActive        = "active"
	StatusFailed        = "failed"
	StatusDisabled      = "disabled"
	StatusIncompatible  = "incompatible"
	StatusMissing       = "missing"
)

type ConfigStatus struct {
	Key         string                  `json:"key"`
	Environment string                  `json:"environment"`
	Type        nativeplugin.ConfigType `json:"type"`
	Required    bool                    `json:"required"`
	Secret      bool                    `json:"secret"`
	Description string                  `json:"description"`
	Configured  bool                    `json:"configured"`
	Source      string                  `json:"source,omitempty"`
}

type ToolStatus struct {
	ID                   string                           `json:"id"`
	Name                 string                           `json:"name"`
	Effect               nativeplugin.Effect              `json:"effect"`
	Identity             nativeplugin.IdentityRequirement `json:"identity"`
	StateScope           nativeplugin.StateScope          `json:"state_scope"`
	ConfirmationRequired bool                             `json:"confirmation_required"`
	Idempotency          nativeplugin.Idempotency         `json:"idempotency"`
}

type Status struct {
	ID                   string                      `json:"id"`
	Version              string                      `json:"version"`
	SDKVersion           int                         `json:"sdk_version"`
	Description          string                      `json:"description"`
	State                string                      `json:"state"`
	StateVersion         uint32                      `json:"state_version"`
	ManifestHash         string                      `json:"manifest_hash"`
	Required             bool                        `json:"required"`
	ManagedByEnvironment bool                        `json:"managed_by_environment"`
	Configuration        []ConfigStatus              `json:"configuration"`
	Tools                []ToolStatus                `json:"tools"`
	Network              []nativeplugin.NetworkClaim `json:"network"`
	Capabilities         []nativeplugin.Capability   `json:"capabilities"`
	LastErrorCode        string                      `json:"last_error_code,omitempty"`
	LastError            string                      `json:"last_error,omitempty"`
}

type Options struct {
	Environment           func(string) (string, bool)
	Logger                *log.Logger
	State                 nativepluginstate.Backend
	IdentityKey           []byte
	Required              []string
	DisabledByEnvironment []string
}

type entry struct {
	plugin       nativeplugin.Plugin
	manifest     nativeplugin.Manifest
	manifestHash string
	config       nativeplugin.Config
	instance     nativeplugin.Instance
	status       Status
	limits       map[string]chan struct{}
	inFlight     sync.WaitGroup
}

type Manager struct {
	mu          sync.RWMutex
	syncMu      sync.Mutex
	entries     map[string]*entry
	environment func(string) (string, bool)
	logger      *log.Logger
	state       nativepluginstate.Backend
	identityKey []byte
	required    map[string]bool
	disabledEnv map[string]bool
}

func New(plugins []nativeplugin.Plugin, options Options) (*Manager, error) {
	if options.Environment == nil {
		options.Environment = func(string) (string, bool) { return "", false }
	}
	if options.Logger == nil {
		options.Logger = log.Default()
	}
	if options.State == nil {
		return nil, errors.New("native plugin state backend is required")
	}
	if len(options.IdentityKey) < 32 {
		return nil, errors.New("native plugin identity projection key must contain at least 32 bytes")
	}
	manager := &Manager{entries: make(map[string]*entry), environment: options.Environment, logger: options.Logger, state: options.State, identityKey: append([]byte(nil), options.IdentityKey...), required: stringSet(options.Required), disabledEnv: stringSet(options.DisabledByEnvironment)}
	for index, plugin := range plugins {
		if plugin == nil {
			return nil, fmt.Errorf("native plugin registration %d is nil", index)
		}
		manifest, err := describe(plugin)
		if err != nil {
			return nil, err
		}
		if err := nativeplugin.ValidateManifest(manifest); err != nil {
			return nil, fmt.Errorf("native plugin %q manifest: %w", manifest.ID, err)
		}
		if _, exists := manager.entries[manifest.ID]; exists {
			return nil, fmt.Errorf("native plugin id %q is registered more than once", manifest.ID)
		}
		hash, err := nativeplugin.ManifestHash(manifest)
		if err != nil {
			return nil, err
		}
		manager.entries[manifest.ID] = &entry{plugin: plugin, manifest: manifest, manifestHash: hash, status: statusFromManifest(manifest, hash), limits: make(map[string]chan struct{})}
	}
	for required := range manager.required {
		if manager.entries[required] == nil {
			return nil, fmt.Errorf("required native plugin %q is not registered", required)
		}
		if manager.disabledEnv[required] {
			return nil, fmt.Errorf("required native plugin %q cannot also be environment-disabled", required)
		}
	}
	for disabled := range manager.disabledEnv {
		if manager.entries[disabled] == nil {
			return nil, fmt.Errorf("environment-disabled native plugin %q is not registered", disabled)
		}
	}
	return manager, nil
}

func describe(plugin nativeplugin.Plugin) (manifest nativeplugin.Manifest, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("native plugin Describe panicked: %v", recovered)
		}
	}()
	first := plugin.Describe()
	second := plugin.Describe()
	left, _ := json.Marshal(first)
	right, _ := json.Marshal(second)
	if string(left) != string(right) {
		return nativeplugin.Manifest{}, errors.New("native plugin Describe is not deterministic")
	}
	return first, nil
}

func statusFromManifest(manifest nativeplugin.Manifest, hash string) Status {
	status := Status{ID: manifest.ID, Version: manifest.Version, SDKVersion: manifest.SDKVersion, Description: manifest.Description, State: StatusDiscovered, StateVersion: manifest.StateVersion, ManifestHash: hash, Network: append([]nativeplugin.NetworkClaim(nil), manifest.Network...), Capabilities: append([]nativeplugin.Capability(nil), manifest.Capabilities...)}
	for _, spec := range manifest.Config {
		status.Configuration = append(status.Configuration, ConfigStatus{Key: spec.Key, Environment: nativeplugin.EnvironmentName(manifest.ID, spec.Key), Type: spec.Type, Required: spec.Required, Secret: spec.Type == nativeplugin.ConfigSecret, Description: spec.Description})
	}
	for _, tool := range manifest.Tools {
		status.Tools = append(status.Tools, ToolStatus{ID: tool.ID, Name: tool.FullName(), Effect: tool.Effect, Identity: tool.Identity, StateScope: tool.StateScope, ConfirmationRequired: tool.ConfirmationRequired, Idempotency: tool.Idempotency})
	}
	return status
}

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var requiredErrors []error
	ids := sortedEntryIDs(m.entries)
	for _, id := range ids {
		entry := m.entries[id]
		entry.status.Required = m.required[id]
		entry.status.ManagedByEnvironment = m.disabledEnv[id]
		if err := m.activateLocked(ctx, entry); err != nil && m.required[id] {
			requiredErrors = append(requiredErrors, fmt.Errorf("required native plugin %s: %w", id, err))
		}
	}
	return errors.Join(requiredErrors...)
}

func (m *Manager) activateLocked(ctx context.Context, entry *entry) error {
	entry.status.LastError, entry.status.LastErrorCode = "", ""
	enabled, err := nativepluginstate.Enabled(ctx, m.state, entry.manifest.ID)
	if err != nil {
		return m.fail(entry, StatusFailed, "state_unavailable", err)
	}
	if m.disabledEnv[entry.manifest.ID] || !enabled {
		entry.status.State = StatusDisabled
		if m.required[entry.manifest.ID] {
			return errors.New("required native plugin is disabled")
		}
		return nil
	}
	config, statuses, err := resolveConfig(entry.manifest, m.environment)
	entry.status.Configuration = statuses
	if err != nil {
		return m.fail(entry, StatusMisconfigured, "configuration_invalid", err)
	}
	entry.config = config
	currentVersion, err := nativepluginstate.CurrentVersion(ctx, m.state, entry.manifest.ID)
	if err != nil {
		return m.fail(entry, StatusFailed, "state_version_unavailable", err)
	}
	if currentVersion > entry.manifest.StateVersion {
		return m.fail(entry, StatusIncompatible, "state_downgrade", fmt.Errorf("stored state version %d is newer than source version %d", currentVersion, entry.manifest.StateVersion))
	}
	entry.status.State = StatusUpgrading
	for currentVersion < entry.manifest.StateVersion {
		next := currentVersion + 1
		upgrader, ok := entry.plugin.(nativeplugin.StateUpgrader)
		if !ok && currentVersion != 0 {
			return m.fail(entry, StatusIncompatible, "state_upgrader_missing", fmt.Errorf("state upgrade %d to %d has no upgrader", currentVersion, next))
		}
		var upgrade func(nativeplugin.UpgradeStore) error
		if ok {
			from := currentVersion
			upgrade = func(store nativeplugin.UpgradeStore) error { return upgrader.UpgradeState(ctx, from, next, store) }
		}
		if err := nativepluginstate.Upgrade(ctx, m.state, entry.manifest.ID, currentVersion, next, upgrade); err != nil {
			return m.fail(entry, StatusFailed, "state_upgrade_failed", err)
		}
		currentVersion = next
	}
	host, err := m.host(entry)
	if err != nil {
		return m.fail(entry, StatusMisconfigured, "network_configuration_invalid", err)
	}
	instance, err := openSafely(ctx, entry.plugin, host)
	if err != nil {
		return m.fail(entry, StatusFailed, "open_failed", err)
	}
	entry.instance = instance
	entry.limits = make(map[string]chan struct{}, len(entry.manifest.Tools))
	for _, tool := range entry.manifest.Tools {
		entry.limits[tool.ID] = make(chan struct{}, tool.MaxConcurrency)
	}
	entry.status.State = StatusActive
	return nil
}

func (m *Manager) fail(entry *entry, state, code string, _ error) error {
	entry.status.State, entry.status.LastErrorCode = state, code
	entry.status.LastError = diagnosticMessage(code)
	m.logger.Printf("native plugin %s activation failed code=%s", entry.manifest.ID, code)
	return fmt.Errorf("native plugin activation failed (%s): %w", code, redactedPluginError)
}

var redactedPluginError = errors.New("plugin diagnostic redacted")

func openSafely(ctx context.Context, plugin nativeplugin.Plugin, host nativeplugin.Host) (instance nativeplugin.Instance, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("plugin Open panicked: %v", recovered)
		}
	}()
	instance, err = plugin.Open(ctx, host)
	if err == nil && instance == nil {
		err = errors.New("plugin Open returned a nil instance")
	}
	return instance, err
}

func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result error
	for _, entry := range m.entries {
		if entry.instance != nil {
			entry.inFlight.Wait()
			if err := closeSafely(ctx, entry.instance); err != nil {
				result = errors.Join(result, errors.New("native plugin close failed; diagnostic redacted"))
			}
			entry.instance = nil
		}
	}
	return result
}

func closeSafely(ctx context.Context, instance nativeplugin.Instance) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("plugin Close panicked: %v", recovered)
		}
	}()
	return instance.Close(ctx)
}

func (m *Manager) Statuses() []Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := sortedEntryIDs(m.entries)
	result := make([]Status, 0, len(ids))
	for _, id := range ids {
		result = append(result, cloneStatus(m.entries[id].status))
	}
	return result
}

func (m *Manager) SetEnabled(ctx context.Context, pluginID string, enabled bool) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[pluginID]
	if entry == nil {
		return Status{}, errors.New("native plugin is not registered")
	}
	if m.disabledEnv[pluginID] && enabled {
		return Status{}, errors.New("native plugin is disabled by deployment environment")
	}
	if m.required[pluginID] && !enabled {
		return Status{}, errors.New("required native plugin cannot be disabled")
	}
	if enabled && entry.status.State == StatusActive && entry.instance != nil {
		return cloneStatus(entry.status), nil
	}
	if !enabled {
		if entry.instance != nil {
			entry.inFlight.Wait()
		}
		if err := nativepluginstate.SetEnabled(ctx, m.state, pluginID, false); err != nil {
			return Status{}, err
		}
		if entry.instance != nil {
			if err := closeSafely(ctx, entry.instance); err != nil {
				m.logger.Printf("native plugin %s close failed during disable; diagnostic redacted", pluginID)
			}
		}
		entry.instance = nil
		entry.status.State, entry.status.LastError, entry.status.LastErrorCode = StatusDisabled, "", ""
		return cloneStatus(entry.status), nil
	}
	if err := nativepluginstate.SetEnabled(ctx, m.state, pluginID, true); err != nil {
		return Status{}, err
	}
	if err := m.activateLocked(ctx, entry); err != nil {
		return cloneStatus(entry.status), err
	}
	return cloneStatus(entry.status), nil
}

func (m *Manager) AvailableNative(tool model.Tool) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.availableLocked(tool)
}

func (m *Manager) ValidateNativeTool(tool model.Tool) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.availableLocked(tool) {
		return errors.New("native plugin is inactive or its source contract does not match this tool revision")
	}
	return nil
}

func (m *Manager) availableLocked(tool model.Tool) bool {
	if tool.BackendKind != "native" || tool.UpstreamDrifted {
		return false
	}
	entry := m.entries[tool.NativePluginID]
	if entry == nil || entry.instance == nil || entry.status.State != StatusActive || entry.manifest.Version != tool.NativePluginVersion || entry.manifest.SDKVersion != tool.NativeSDKVersion || entry.manifestHash != tool.NativeManifestHash {
		return false
	}
	spec, ok := entry.manifest.Tool(tool.NativeToolID)
	if !ok {
		return false
	}
	hash, err := nativeplugin.ToolContractHash(spec)
	return err == nil && hash == tool.NativeContractHash
}

func (m *Manager) ExecuteNative(ctx context.Context, tool model.Tool, arguments map[string]any, principal toolruntime.Principal) (result any, err error) {
	m.mu.RLock()
	entry := m.entries[tool.NativePluginID]
	if entry == nil || !m.availableLocked(tool) {
		m.mu.RUnlock()
		return nil, toolruntime.ErrDenied
	}
	instance := entry.instance
	spec, _ := entry.manifest.Tool(tool.NativeToolID)
	limit := entry.limits[spec.ID]
	pluginID := entry.manifest.ID
	entry.inFlight.Add(1)
	defer entry.inFlight.Done()
	m.mu.RUnlock()
	select {
	case limit <- struct{}{}:
		defer func() { <-limit }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	identity, internalScopes, err := m.identity(pluginID, spec.Identity, principal)
	if err != nil {
		return nil, toolruntime.ErrDenied
	}
	scopeID := ""
	switch spec.StateScope {
	case nativeplugin.StateActor:
		scopeID = internalScopes.actor
	case nativeplugin.StateCustomer:
		scopeID = internalScopes.customer
	case nativeplugin.StateInstallation:
		scopeID = internalScopes.installation
	}
	state := nativepluginstate.Bind(m.state, pluginID, string(spec.StateScope), scopeID)
	callCtx, cancel := context.WithTimeout(ctx, spec.Timeout)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			m.logger.Printf("native plugin %s tool %s panic recovered", pluginID, spec.ID)
			result, err = nil, nativeplugin.Fail(nativeplugin.ErrorInternal, "Native tool failed safely", nil)
		}
	}()
	value, err := instance.Invoke(callCtx, nativeplugin.Invocation{ToolID: spec.ID, Arguments: arguments, Identity: identity, State: state, RequestID: principal.RequestID, IdempotencyKey: principal.IdempotencyKey})
	if err != nil {
		return nil, safePluginCallError(err)
	}
	if value.Structured == nil {
		return nil, nativeplugin.Fail(nativeplugin.ErrorInternal, "Native tool returned an invalid result", errors.New("structured result is nil"))
	}
	encoded, err := json.Marshal(value.Structured)
	if err != nil || int64(len(encoded)) > spec.MaxResultBytes {
		return nil, nativeplugin.Fail(nativeplugin.ErrorInternal, "Native tool result exceeded its declared contract", err)
	}
	if err := toolruntime.ValidateArguments(spec.OutputSchema, value.Structured); err != nil {
		return nil, nativeplugin.Fail(nativeplugin.ErrorInternal, "Native tool returned an invalid result", err)
	}
	return value.Structured, nil
}

func safePluginCallError(err error) error {
	var callError *nativeplugin.CallError
	if !errors.As(err, &callError) || callError == nil || !validPluginErrorCode(callError.Code) {
		return nativeplugin.Fail(nativeplugin.ErrorInternal, "Native tool failed safely", nil)
	}
	message := strings.TrimSpace(callError.SafeMessage)
	if message == "" || len(message) > 300 || !utf8.ValidString(message) || strings.IndexFunc(message, unicode.IsControl) >= 0 {
		return nativeplugin.Fail(nativeplugin.ErrorInternal, "Native tool failed safely", nil)
	}
	return nativeplugin.Fail(callError.Code, message, nil)
}

func validPluginErrorCode(code nativeplugin.ErrorCode) bool {
	switch code {
	case nativeplugin.ErrorInvalidArgument, nativeplugin.ErrorNotFound, nativeplugin.ErrorConflict, nativeplugin.ErrorUnauthorized, nativeplugin.ErrorRateLimited, nativeplugin.ErrorTemporary, nativeplugin.ErrorInternal:
		return true
	default:
		return false
	}
}

type scopeIDs struct{ actor, customer, installation string }

func (m *Manager) identity(pluginID string, requirement nativeplugin.IdentityRequirement, principal toolruntime.Principal) (nativeplugin.IdentityView, scopeIDs, error) {
	ids := scopeIDs{}
	view := nativeplugin.IdentityView{}
	if principal.Subject != "" {
		ids.actor = m.opaqueID(pluginID, "actor", principal.Issuer+"\x00"+principal.Subject)
		view.Actor = &nativeplugin.IdentityRef{ID: "act_" + ids.actor}
	}
	if principal.CustomerAccountID != "" {
		ids.customer = m.opaqueID(pluginID, "customer", principal.CustomerAccountID)
		view.Customer = &nativeplugin.IdentityRef{ID: "cus_" + ids.customer}
	}
	if principal.InstallationID != "" {
		ids.installation = m.opaqueID(pluginID, "installation", principal.InstallationID)
		view.Installation = &nativeplugin.IdentityRef{ID: "ins_" + ids.installation}
	}
	switch requirement {
	case nativeplugin.IdentityNone:
		return nativeplugin.IdentityView{}, ids, nil
	case nativeplugin.IdentityOptional:
		return view, ids, nil
	case nativeplugin.IdentityActorRequired:
		if view.Actor == nil {
			return nativeplugin.IdentityView{}, scopeIDs{}, errors.New("actor identity is required")
		}
		view.Customer, view.Installation = nil, nil
	case nativeplugin.IdentityCustomerRequired:
		if view.Customer == nil {
			return nativeplugin.IdentityView{}, scopeIDs{}, errors.New("customer identity is required")
		}
		view.Actor, view.Installation = nil, nil
	case nativeplugin.IdentityActorAndCustomerRequired:
		if view.Actor == nil || view.Customer == nil {
			return nativeplugin.IdentityView{}, scopeIDs{}, errors.New("actor and customer identity are required")
		}
		view.Installation = nil
	case nativeplugin.IdentityInstallationRequired:
		if view.Installation == nil {
			return nativeplugin.IdentityView{}, scopeIDs{}, errors.New("installation identity is required")
		}
		view.Actor, view.Customer = nil, nil
	default:
		return nativeplugin.IdentityView{}, scopeIDs{}, errors.New("identity requirement is invalid")
	}
	return view, ids, nil
}

func (m *Manager) opaqueID(pluginID, kind, source string) string {
	mac := hmac.New(sha256.New, m.identityKey)
	_, _ = mac.Write([]byte("dokosoko-native-identity-v1\x00" + pluginID + "\x00" + kind + "\x00" + source))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:18])
}

func resolveConfig(manifest nativeplugin.Manifest, environment func(string) (string, bool)) (nativeplugin.Config, []ConfigStatus, error) {
	values := make(map[string]nativeplugin.ConfigValue, len(manifest.Config))
	statuses := make([]ConfigStatus, 0, len(manifest.Config))
	var problems []string
	for _, spec := range manifest.Config {
		envName := nativeplugin.EnvironmentName(manifest.ID, spec.Key)
		raw, configured := environment(envName)
		configured = configured && raw != ""
		status := ConfigStatus{Key: spec.Key, Environment: envName, Type: spec.Type, Required: spec.Required, Secret: spec.Type == nativeplugin.ConfigSecret, Description: spec.Description, Configured: configured}
		if configured {
			status.Source = "environment"
		}
		statuses = append(statuses, status)
		if !configured {
			if spec.Required {
				problems = append(problems, envName+" is required")
			}
			continue
		}
		validated := raw
		if spec.Type != nativeplugin.ConfigString && spec.Type != nativeplugin.ConfigSecret {
			validated = strings.TrimSpace(raw)
		}
		if err := validateConfigValue(spec.Type, validated); err != nil {
			problems = append(problems, envName+": "+err.Error())
			continue
		}
		value := nativeplugin.ConfigValue{Type: spec.Type, String: validated}
		if spec.Type == nativeplugin.ConfigSecret {
			value.String, value.Secret = "", nativeplugin.NewSecret(validated)
		}
		values[spec.Key] = value
	}
	if len(problems) > 0 {
		return nativeplugin.Config{}, statuses, errors.New(strings.Join(problems, "; "))
	}
	return nativeplugin.NewConfig(values), statuses, nil
}

func validateConfigValue(kind nativeplugin.ConfigType, value string) error {
	switch kind {
	case nativeplugin.ConfigBoolean:
		_, err := strconv.ParseBool(value)
		return err
	case nativeplugin.ConfigInteger:
		_, err := strconv.ParseInt(value, 10, 64)
		return err
	case nativeplugin.ConfigDuration:
		_, err := time.ParseDuration(value)
		return err
	case nativeplugin.ConfigURL:
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || net.ParseIP(parsed.Hostname()) != nil || parsed.Port() != "" && parsed.Port() != "443" {
			return errors.New("must be a fixed credential-free HTTPS URL on port 443 without query or fragment")
		}
	}
	return nil
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result[value] = true
		}
	}
	return result
}

func sortedEntryIDs(entries map[string]*entry) []string {
	result := make([]string, 0, len(entries))
	for id := range entries {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func cloneStatus(value Status) Status {
	value.Configuration = append([]ConfigStatus(nil), value.Configuration...)
	value.Tools = append([]ToolStatus(nil), value.Tools...)
	value.Network = append([]nativeplugin.NetworkClaim(nil), value.Network...)
	value.Capabilities = append([]nativeplugin.Capability(nil), value.Capabilities...)
	return value
}

func diagnosticMessage(code string) string {
	switch code {
	case "configuration_invalid":
		return "Required configuration is missing or invalid."
	case "state_downgrade":
		return "Stored plugin state is newer than this source version."
	case "state_upgrader_missing", "state_upgrade_failed":
		return "Plugin state could not be upgraded safely."
	case "network_configuration_invalid":
		return "Declared network configuration is invalid."
	case "open_failed":
		return "The plugin could not be opened safely."
	default:
		return "The plugin could not be activated safely."
	}
}

func nativeToolID(pluginID, toolID string) string {
	digest := sha256.Sum256([]byte("dokosoko-native-tool-v1\x00" + pluginID + "\x00" + toolID))
	bytes := append([]byte(nil), digest[:16]...)
	bytes[6] = bytes[6]&0x0f | 0x50
	bytes[8] = bytes[8]&0x3f | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

var _ toolruntime.NativeExecutor = (*Manager)(nil)
