package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	CurrentVersion        = 1
	defaultUploadMaxBytes = int64(5_000_000)
	maxConfigBytes        = int64(1 << 20)
)

type Source string

const (
	SourceBuiltIn           Source = "built_in"
	SourceConfigurationFile Source = "configuration_file"
	SourceEnvironment       Source = "environment"
)

type Item struct {
	Key             string `json:"key"`
	Source          Source `json:"source"`
	Value           string `json:"value,omitempty"`
	Sensitive       bool   `json:"sensitive"`
	Configured      bool   `json:"configured"`
	RestartRequired bool   `json:"restart_required"`
}

type Status struct {
	Version               int    `json:"version"`
	ConfigurationFile     string `json:"configuration_file,omitempty"`
	ChangesRequireRestart bool   `json:"changes_require_restart"`
	Items                 []Item `json:"items"`
}

type Analysis struct {
	Model            string
	MaxInputTokens   *int
	MaxOutputTokens  *int
	DailyTokenBudget *int64
}

type AI struct {
	Provider string
	APIKey   string
	Endpoint string
	Analysis Analysis
}

type Crawler struct {
	MaxPages                 int
	MaxBytes                 int
	DataDirectory            string
	UploadDirectory          string
	AllowLocalhostSubdomains bool
	LocalhostPorts           []int
}

type ControlPlaneIdentity struct {
	Name *string `json:"name,omitempty"`
	Slug *string `json:"slug,omitempty"`
}

type ControlPlaneDeployment struct {
	Name                  *string `json:"name,omitempty"`
	Slug                  *string `json:"slug,omitempty"`
	Description           *string `json:"description,omitempty"`
	FeedbackSubmissionURL *string `json:"feedback_submission_url,omitempty"`
	ErrorSubmissionURL    *string `json:"error_submission_url,omitempty"`
}

type ControlPlaneEnvironment struct {
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	IsProduction bool   `json:"is_production"`
}

type ControlPlane struct {
	Organisation ControlPlaneIdentity       `json:"organisation"`
	Deployment   ControlPlaneDeployment     `json:"deployment"`
	Environments *[]ControlPlaneEnvironment `json:"environments,omitempty"`
}

type Config struct {
	Listen                string
	PublicURL             string
	UIDirectory           string
	DatabaseURL           string
	MigrationsDirectory   string
	MasterKey             string
	SetupToken            string
	DevMemory             bool
	AllowDemoTokens       bool
	AllowInsecureHTTP     bool
	UploadDirectory       string
	UploadMaxBytes        int64
	ToolLocalhostHosts    []string
	NativePluginsRequired []string
	NativePluginsDisabled []string
	AI                    AI
	Crawler               Crawler
	ControlPlane          ControlPlane
	Status                Status
}

type Options struct {
	LookupEnv        func(string) (string, bool)
	ReadFile         func(string) ([]byte, error)
	WorkingDirectory string
}

type secretReference struct {
	Env  string `json:"env,omitempty"`
	File string `json:"file,omitempty"`
}

type fileConfig struct {
	Schema        string            `json:"$schema"`
	Version       int               `json:"version"`
	Server        fileServer        `json:"server"`
	Database      fileDatabase      `json:"database"`
	Security      fileSecurity      `json:"security"`
	Uploads       fileUploads       `json:"uploads"`
	Tools         fileTools         `json:"tools"`
	NativePlugins fileNativePlugins `json:"native_plugins"`
	AI            fileAI            `json:"ai"`
	Crawler       fileCrawler       `json:"crawler"`
	ControlPlane  ControlPlane      `json:"control_plane"`
}

type fileServer struct {
	Listen      *string `json:"listen"`
	PublicURL   *string `json:"public_url"`
	UIDirectory *string `json:"ui_directory"`
}

type fileDatabase struct {
	URL                 *secretReference `json:"url"`
	MigrationsDirectory *string          `json:"migrations_directory"`
}

type fileSecurity struct {
	MasterKey         *secretReference `json:"master_key"`
	SetupToken        *secretReference `json:"setup_token"`
	DevMemory         *bool            `json:"dev_memory"`
	AllowDemoTokens   *bool            `json:"allow_demo_tokens"`
	AllowInsecureHTTP *bool            `json:"allow_insecure_http"`
}

type fileUploads struct {
	Directory *string `json:"directory"`
	MaxBytes  *int64  `json:"max_bytes"`
}

type fileTools struct {
	LocalhostHosts *[]string `json:"localhost_hosts"`
}

type fileNativePlugins struct {
	Required *[]string `json:"required"`
	Disabled *[]string `json:"disabled"`
}

type fileAI struct {
	Provider *string          `json:"provider"`
	APIKey   *secretReference `json:"api_key"`
	Endpoint *string          `json:"endpoint"`
	Analysis fileAnalysis     `json:"analysis"`
}

type fileAnalysis struct {
	Model            *string `json:"model"`
	MaxInputTokens   *int    `json:"max_input_tokens"`
	MaxOutputTokens  *int    `json:"max_output_tokens"`
	DailyTokenBudget *int64  `json:"daily_token_budget"`
}

type fileCrawler struct {
	MaxPages                 *int    `json:"max_pages"`
	MaxBytes                 *int    `json:"max_bytes"`
	DataDirectory            *string `json:"data_directory"`
	UploadDirectory          *string `json:"upload_directory"`
	AllowLocalhostSubdomains *bool   `json:"allow_localhost_subdomains"`
	LocalhostPorts           *[]int  `json:"localhost_ports"`
}

type tracker struct {
	order   []string
	items   map[string]Item
	fileDir string
	options Options
}

func Load() (Config, error) {
	return LoadWithOptions(Options{})
}

func LoadWithOptions(options Options) (Config, error) {
	if options.LookupEnv == nil {
		options.LookupEnv = os.LookupEnv
	}
	if options.ReadFile == nil {
		options.ReadFile = os.ReadFile
	}
	if options.WorkingDirectory == "" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return Config{}, fmt.Errorf("resolve working directory: %w", err)
		}
		options.WorkingDirectory = workingDirectory
	}

	value := defaultConfig()
	state := newTracker(options)
	state.recordDefaults(value)
	configPath := strings.TrimSpace(environmentValue(options.LookupEnv, "DOKOSOKO_CONFIG_FILE"))
	if configPath != "" {
		absolute, err := absolutePath(configPath, options.WorkingDirectory)
		if err != nil {
			return Config{}, fmt.Errorf("resolve DOKOSOKO_CONFIG_FILE: %w", err)
		}
		fileValue, err := readConfigurationFile(absolute, options.ReadFile)
		if err != nil {
			return Config{}, err
		}
		state.fileDir = filepath.Dir(absolute)
		if err := state.applyFile(&value, fileValue); err != nil {
			return Config{}, err
		}
		value.Status.ConfigurationFile = absolute
	}
	if err := state.applyEnvironment(&value); err != nil {
		return Config{}, err
	}
	if err := validate(value); err != nil {
		return Config{}, err
	}
	value.Status.Version = CurrentVersion
	value.Status.ChangesRequireRestart = true
	value.Status.Items = state.list()
	return value, nil
}

func defaultConfig() Config {
	return Config{
		Listen:              ":8080",
		PublicURL:           "http://localhost:8080",
		UIDirectory:         "./dist/client",
		MigrationsDirectory: "./migrations",
		UploadMaxBytes:      defaultUploadMaxBytes,
		Crawler: Crawler{
			MaxPages:       500,
			MaxBytes:       5_000_000,
			DataDirectory:  "/data",
			LocalhostPorts: []int{80, 443},
		},
	}
}

func newTracker(options Options) *tracker {
	return &tracker{items: make(map[string]Item), options: options}
}

func (t *tracker) set(key string, source Source, value string, sensitive, configured bool) {
	if _, exists := t.items[key]; !exists {
		t.order = append(t.order, key)
	}
	if sensitive {
		value = ""
	}
	t.items[key] = Item{Key: key, Source: source, Value: value, Sensitive: sensitive, Configured: configured, RestartRequired: true}
}

func (t *tracker) recordDefaults(value Config) {
	t.set("server.listen", SourceBuiltIn, value.Listen, false, true)
	t.set("server.public_url", SourceBuiltIn, value.PublicURL, false, true)
	t.set("server.ui_directory", SourceBuiltIn, value.UIDirectory, false, true)
	t.set("database.url", SourceBuiltIn, "", true, false)
	t.set("database.migrations_directory", SourceBuiltIn, value.MigrationsDirectory, false, true)
	t.set("security.master_key", SourceBuiltIn, "", true, false)
	t.set("security.setup_token", SourceBuiltIn, "", true, false)
	t.set("security.dev_memory", SourceBuiltIn, "false", false, true)
	t.set("security.allow_demo_tokens", SourceBuiltIn, "false", false, true)
	t.set("security.allow_insecure_http", SourceBuiltIn, "false", false, true)
	t.set("uploads.directory", SourceBuiltIn, "", false, false)
	t.set("uploads.max_bytes", SourceBuiltIn, strconv.FormatInt(value.UploadMaxBytes, 10), false, true)
	t.set("tools.localhost_hosts", SourceBuiltIn, "", false, false)
	t.set("native_plugins.required", SourceBuiltIn, "", false, false)
	t.set("native_plugins.disabled", SourceBuiltIn, "", false, false)
	t.set("ai.provider", SourceBuiltIn, "", false, false)
	t.set("ai.api_key", SourceBuiltIn, "", true, false)
	t.set("ai.endpoint", SourceBuiltIn, "", false, false)
	t.set("ai.analysis.model", SourceBuiltIn, "", false, false)
	t.set("ai.analysis.max_input_tokens", SourceBuiltIn, "", false, false)
	t.set("ai.analysis.max_output_tokens", SourceBuiltIn, "", false, false)
	t.set("ai.analysis.daily_token_budget", SourceBuiltIn, "", false, false)
	t.set("control_plane.organisation.name", SourceBuiltIn, "", false, false)
	t.set("control_plane.organisation.slug", SourceBuiltIn, "", false, false)
	t.set("control_plane.deployment.name", SourceBuiltIn, "", false, false)
	t.set("control_plane.deployment.slug", SourceBuiltIn, "", false, false)
	t.set("control_plane.deployment.description", SourceBuiltIn, "", false, false)
	t.set("control_plane.deployment.feedback_submission_url", SourceBuiltIn, "", false, false)
	t.set("control_plane.deployment.error_submission_url", SourceBuiltIn, "", false, false)
	t.set("control_plane.environments", SourceBuiltIn, "", false, false)
	t.set("crawler.max_pages", SourceBuiltIn, strconv.Itoa(value.Crawler.MaxPages), false, true)
	t.set("crawler.max_bytes", SourceBuiltIn, strconv.Itoa(value.Crawler.MaxBytes), false, true)
	t.set("crawler.data_directory", SourceBuiltIn, value.Crawler.DataDirectory, false, true)
	t.set("crawler.upload_directory", SourceBuiltIn, "", false, false)
	t.set("crawler.allow_localhost_subdomains", SourceBuiltIn, "false", false, true)
	t.set("crawler.localhost_ports", SourceBuiltIn, "80,443", false, true)
}

func readConfigurationFile(path string, readFile func(string) ([]byte, error)) (fileConfig, error) {
	contents, err := readFile(path)
	if err != nil {
		return fileConfig{}, fmt.Errorf("read configuration file: %w", err)
	}
	if int64(len(contents)) > maxConfigBytes {
		return fileConfig{}, errors.New("configuration file must be 1 MiB or smaller")
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var value fileConfig
	if err := decoder.Decode(&value); err != nil {
		return fileConfig{}, fmt.Errorf("decode configuration file: %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return fileConfig{}, err
	}
	if value.Version != CurrentVersion {
		return fileConfig{}, fmt.Errorf("configuration version must be %d", CurrentVersion)
	}
	return value, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode configuration file: %w", err)
	}
	return errors.New("configuration file must contain one JSON object")
}

func (t *tracker) applyFile(value *Config, fileValue fileConfig) error {
	applyString := func(key string, target *string, configured *string, path bool) error {
		if configured == nil {
			return nil
		}
		candidate := strings.TrimSpace(*configured)
		if path && candidate != "" {
			absolute, err := absolutePath(candidate, t.fileDir)
			if err != nil {
				return fmt.Errorf("resolve %s: %w", key, err)
			}
			candidate = absolute
		}
		*target = candidate
		t.set(key, SourceConfigurationFile, candidate, false, candidate != "")
		return nil
	}
	if err := applyString("server.listen", &value.Listen, fileValue.Server.Listen, false); err != nil {
		return err
	}
	if err := applyString("server.public_url", &value.PublicURL, fileValue.Server.PublicURL, false); err != nil {
		return err
	}
	if err := applyString("server.ui_directory", &value.UIDirectory, fileValue.Server.UIDirectory, true); err != nil {
		return err
	}
	if err := applyString("database.migrations_directory", &value.MigrationsDirectory, fileValue.Database.MigrationsDirectory, true); err != nil {
		return err
	}
	if err := applyString("uploads.directory", &value.UploadDirectory, fileValue.Uploads.Directory, true); err != nil {
		return err
	}
	if err := applyString("ai.provider", &value.AI.Provider, fileValue.AI.Provider, false); err != nil {
		return err
	}
	if err := applyString("ai.endpoint", &value.AI.Endpoint, fileValue.AI.Endpoint, false); err != nil {
		return err
	}
	if err := applyString("ai.analysis.model", &value.AI.Analysis.Model, fileValue.AI.Analysis.Model, false); err != nil {
		return err
	}
	if err := applyString("crawler.data_directory", &value.Crawler.DataDirectory, fileValue.Crawler.DataDirectory, true); err != nil {
		return err
	}
	if err := applyString("crawler.upload_directory", &value.Crawler.UploadDirectory, fileValue.Crawler.UploadDirectory, true); err != nil {
		return err
	}
	if fileValue.Crawler.UploadDirectory == nil && fileValue.Uploads.Directory != nil {
		value.Crawler.UploadDirectory = value.UploadDirectory
		t.set("crawler.upload_directory", SourceConfigurationFile, value.Crawler.UploadDirectory, false, value.Crawler.UploadDirectory != "")
	}
	applyManagedString := func(key string, target **string, configured *string) {
		if configured == nil {
			return
		}
		candidate := strings.TrimSpace(*configured)
		*target = &candidate
		t.set(key, SourceConfigurationFile, candidate, false, true)
	}
	applyManagedString("control_plane.organisation.name", &value.ControlPlane.Organisation.Name, fileValue.ControlPlane.Organisation.Name)
	applyManagedString("control_plane.organisation.slug", &value.ControlPlane.Organisation.Slug, fileValue.ControlPlane.Organisation.Slug)
	applyManagedString("control_plane.deployment.name", &value.ControlPlane.Deployment.Name, fileValue.ControlPlane.Deployment.Name)
	applyManagedString("control_plane.deployment.slug", &value.ControlPlane.Deployment.Slug, fileValue.ControlPlane.Deployment.Slug)
	applyManagedString("control_plane.deployment.description", &value.ControlPlane.Deployment.Description, fileValue.ControlPlane.Deployment.Description)
	applyManagedString("control_plane.deployment.feedback_submission_url", &value.ControlPlane.Deployment.FeedbackSubmissionURL, fileValue.ControlPlane.Deployment.FeedbackSubmissionURL)
	applyManagedString("control_plane.deployment.error_submission_url", &value.ControlPlane.Deployment.ErrorSubmissionURL, fileValue.ControlPlane.Deployment.ErrorSubmissionURL)
	if fileValue.ControlPlane.Environments != nil {
		environments := append([]ControlPlaneEnvironment(nil), (*fileValue.ControlPlane.Environments)...)
		value.ControlPlane.Environments = &environments
		t.set("control_plane.environments", SourceConfigurationFile, environmentSummary(environments), false, true)
	}

	var err error
	if value.DatabaseURL, err = t.secret("database.url", fileValue.Database.URL); err != nil {
		return err
	}
	if value.MasterKey, err = t.secret("security.master_key", fileValue.Security.MasterKey); err != nil {
		return err
	}
	if value.SetupToken, err = t.secret("security.setup_token", fileValue.Security.SetupToken); err != nil {
		return err
	}
	if value.AI.APIKey, err = t.secret("ai.api_key", fileValue.AI.APIKey); err != nil {
		return err
	}

	applyBoolFile := func(key string, target *bool, configured *bool) {
		if configured != nil {
			*target = *configured
			t.set(key, SourceConfigurationFile, strconv.FormatBool(*configured), false, true)
		}
	}
	applyBoolFile("security.dev_memory", &value.DevMemory, fileValue.Security.DevMemory)
	applyBoolFile("security.allow_demo_tokens", &value.AllowDemoTokens, fileValue.Security.AllowDemoTokens)
	applyBoolFile("security.allow_insecure_http", &value.AllowInsecureHTTP, fileValue.Security.AllowInsecureHTTP)
	applyBoolFile("crawler.allow_localhost_subdomains", &value.Crawler.AllowLocalhostSubdomains, fileValue.Crawler.AllowLocalhostSubdomains)

	if fileValue.Uploads.MaxBytes != nil {
		value.UploadMaxBytes = *fileValue.Uploads.MaxBytes
		t.set("uploads.max_bytes", SourceConfigurationFile, strconv.FormatInt(value.UploadMaxBytes, 10), false, true)
	}
	applyIntFile := func(key string, target *int, configured *int) {
		if configured != nil {
			*target = *configured
			t.set(key, SourceConfigurationFile, strconv.Itoa(*configured), false, true)
		}
	}
	applyIntFile("crawler.max_pages", &value.Crawler.MaxPages, fileValue.Crawler.MaxPages)
	applyIntFile("crawler.max_bytes", &value.Crawler.MaxBytes, fileValue.Crawler.MaxBytes)
	if fileValue.AI.Analysis.MaxInputTokens != nil {
		candidate := *fileValue.AI.Analysis.MaxInputTokens
		value.AI.Analysis.MaxInputTokens = &candidate
		t.set("ai.analysis.max_input_tokens", SourceConfigurationFile, strconv.Itoa(candidate), false, true)
	}
	if fileValue.AI.Analysis.MaxOutputTokens != nil {
		candidate := *fileValue.AI.Analysis.MaxOutputTokens
		value.AI.Analysis.MaxOutputTokens = &candidate
		t.set("ai.analysis.max_output_tokens", SourceConfigurationFile, strconv.Itoa(candidate), false, true)
	}
	if fileValue.AI.Analysis.DailyTokenBudget != nil {
		candidate := *fileValue.AI.Analysis.DailyTokenBudget
		value.AI.Analysis.DailyTokenBudget = &candidate
		t.set("ai.analysis.daily_token_budget", SourceConfigurationFile, strconv.FormatInt(candidate, 10), false, true)
	}
	applyStringsFile := func(key string, target *[]string, configured *[]string) {
		if configured != nil {
			*target = cleanStrings(*configured)
			t.set(key, SourceConfigurationFile, strings.Join(*target, ","), false, len(*target) > 0)
		}
	}
	applyStringsFile("tools.localhost_hosts", &value.ToolLocalhostHosts, fileValue.Tools.LocalhostHosts)
	applyStringsFile("native_plugins.required", &value.NativePluginsRequired, fileValue.NativePlugins.Required)
	applyStringsFile("native_plugins.disabled", &value.NativePluginsDisabled, fileValue.NativePlugins.Disabled)
	if fileValue.Crawler.LocalhostPorts != nil {
		value.Crawler.LocalhostPorts = append([]int(nil), (*fileValue.Crawler.LocalhostPorts)...)
		t.set("crawler.localhost_ports", SourceConfigurationFile, joinInts(value.Crawler.LocalhostPorts), false, len(value.Crawler.LocalhostPorts) > 0)
	}
	return nil
}

func (t *tracker) secret(key string, reference *secretReference) (string, error) {
	if reference == nil {
		return "", nil
	}
	environmentName := strings.TrimSpace(reference.Env)
	fileName := strings.TrimSpace(reference.File)
	if (environmentName == "") == (fileName == "") {
		return "", fmt.Errorf("%s must reference exactly one environment variable or file", key)
	}
	var value string
	if environmentName != "" {
		value = strings.TrimSpace(environmentValue(t.options.LookupEnv, environmentName))
	} else {
		path, err := absolutePath(fileName, t.fileDir)
		if err != nil {
			return "", fmt.Errorf("resolve %s secret file: %w", key, err)
		}
		contents, err := t.options.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s secret file: %w", key, err)
		}
		value = strings.TrimSpace(string(contents))
	}
	t.set(key, SourceConfigurationFile, "", true, value != "")
	return value, nil
}

func (t *tracker) applyEnvironment(value *Config) error {
	applyString := func(key, name string, target *string, path bool) error {
		candidate, ok := nonEmptyEnvironment(t.options.LookupEnv, name)
		if !ok {
			return nil
		}
		if path {
			absolute, err := absolutePath(candidate, t.options.WorkingDirectory)
			if err != nil {
				return fmt.Errorf("resolve %s: %w", name, err)
			}
			candidate = absolute
		}
		*target = candidate
		t.set(key, SourceEnvironment, candidate, false, true)
		return nil
	}
	if err := applyString("server.listen", "DOKOSOKO_LISTEN", &value.Listen, false); err != nil {
		return err
	}
	if err := applyString("server.public_url", "DOKOSOKO_PUBLIC_URL", &value.PublicURL, false); err != nil {
		return err
	}
	if err := applyString("server.ui_directory", "DOKOSOKO_UI_DIR", &value.UIDirectory, true); err != nil {
		return err
	}
	if err := applyString("database.migrations_directory", "DOKOSOKO_MIGRATIONS_DIR", &value.MigrationsDirectory, true); err != nil {
		return err
	}
	if err := applyString("uploads.directory", "DOKOSOKO_UPLOAD_DIR", &value.UploadDirectory, true); err != nil {
		return err
	}
	if err := applyString("ai.provider", "DOKOSOKO_AI_PROVIDER", &value.AI.Provider, false); err != nil {
		return err
	}
	if err := applyString("ai.endpoint", "DOKOSOKO_AI_ENDPOINT", &value.AI.Endpoint, false); err != nil {
		return err
	}
	if err := applyString("ai.analysis.model", "DOKOSOKO_AI_MODEL_ANALYSIS", &value.AI.Analysis.Model, false); err != nil {
		return err
	}
	if err := applyString("crawler.data_directory", "DOKOSOKO_DATA_DIR", &value.Crawler.DataDirectory, true); err != nil {
		return err
	}
	if err := applyString("crawler.upload_directory", "DOKOSOKO_CRAWLER_UPLOAD_DIR", &value.Crawler.UploadDirectory, true); err != nil {
		return err
	}
	applyManagedEnvironment := func(key, name string, target **string) {
		if candidate, ok := nonEmptyEnvironment(t.options.LookupEnv, name); ok {
			*target = &candidate
			t.set(key, SourceEnvironment, candidate, false, true)
		}
	}
	applyManagedEnvironment("control_plane.organisation.name", "DOKOSOKO_ORGANISATION_NAME", &value.ControlPlane.Organisation.Name)
	applyManagedEnvironment("control_plane.organisation.slug", "DOKOSOKO_ORGANISATION_SLUG", &value.ControlPlane.Organisation.Slug)
	applyManagedEnvironment("control_plane.deployment.name", "DOKOSOKO_DEPLOYMENT_NAME", &value.ControlPlane.Deployment.Name)
	applyManagedEnvironment("control_plane.deployment.slug", "DOKOSOKO_DEPLOYMENT_SLUG", &value.ControlPlane.Deployment.Slug)
	applyManagedEnvironment("control_plane.deployment.description", "DOKOSOKO_DEPLOYMENT_DESCRIPTION", &value.ControlPlane.Deployment.Description)
	applyManagedEnvironment("control_plane.deployment.feedback_submission_url", "DOKOSOKO_DEPLOYMENT_FEEDBACK_SUBMISSION_URL", &value.ControlPlane.Deployment.FeedbackSubmissionURL)
	applyManagedEnvironment("control_plane.deployment.error_submission_url", "DOKOSOKO_DEPLOYMENT_ERROR_SUBMISSION_URL", &value.ControlPlane.Deployment.ErrorSubmissionURL)
	if _, ok := nonEmptyEnvironment(t.options.LookupEnv, "DOKOSOKO_CRAWLER_UPLOAD_DIR"); !ok {
		if candidate, uploadSet := nonEmptyEnvironment(t.options.LookupEnv, "DOKOSOKO_UPLOAD_DIR"); uploadSet {
			absolute, err := absolutePath(candidate, t.options.WorkingDirectory)
			if err != nil {
				return fmt.Errorf("resolve DOKOSOKO_UPLOAD_DIR: %w", err)
			}
			value.Crawler.UploadDirectory = absolute
			t.set("crawler.upload_directory", SourceEnvironment, absolute, false, true)
		}
	}

	var err error
	if value.DatabaseURL, err = t.environmentSecret("database.url", "DOKOSOKO_DATABASE_URL", "DOKOSOKO_DATABASE_URL_FILE", value.DatabaseURL); err != nil {
		return err
	}
	if value.MasterKey, err = t.environmentSecret("security.master_key", "DOKOSOKO_MASTER_KEY", "DOKOSOKO_MASTER_KEY_FILE", value.MasterKey); err != nil {
		return err
	}
	if value.SetupToken, err = t.environmentSecret("security.setup_token", "DOKOSOKO_SETUP_TOKEN", "DOKOSOKO_SETUP_TOKEN_FILE", value.SetupToken); err != nil {
		return err
	}
	if value.AI.APIKey, err = t.environmentSecret("ai.api_key", "DOKOSOKO_AI_API_KEY", "DOKOSOKO_AI_API_KEY_FILE", value.AI.APIKey); err != nil {
		return err
	}

	applyBool := func(key, name string, target *bool) error {
		candidate, ok := nonEmptyEnvironment(t.options.LookupEnv, name)
		if !ok {
			return nil
		}
		parsed, ok := parseBoolean(candidate)
		if !ok {
			return fmt.Errorf("%s must be true or false", name)
		}
		*target = parsed
		t.set(key, SourceEnvironment, strconv.FormatBool(parsed), false, true)
		return nil
	}
	if err := applyBool("security.dev_memory", "DOKOSOKO_DEV_MEMORY", &value.DevMemory); err != nil {
		return err
	}
	if err := applyBool("security.allow_demo_tokens", "DOKOSOKO_ALLOW_DEMO_TOKENS", &value.AllowDemoTokens); err != nil {
		return err
	}
	if err := applyBool("security.allow_insecure_http", "DOKOSOKO_ALLOW_INSECURE_HTTP", &value.AllowInsecureHTTP); err != nil {
		return err
	}
	if err := applyBool("crawler.allow_localhost_subdomains", "DOKOSOKO_CRAWLER_ALLOW_LOCALHOST_SUBDOMAINS", &value.Crawler.AllowLocalhostSubdomains); err != nil {
		return err
	}

	applyPositiveInt := func(key, name string, target *int) error {
		candidate, ok := nonEmptyEnvironment(t.options.LookupEnv, name)
		if !ok {
			return nil
		}
		parsed, err := strconv.Atoi(candidate)
		if err != nil || parsed < 1 {
			return fmt.Errorf("%s must be a positive integer", name)
		}
		*target = parsed
		t.set(key, SourceEnvironment, strconv.Itoa(parsed), false, true)
		return nil
	}
	if candidate, ok := nonEmptyEnvironment(t.options.LookupEnv, "DOKOSOKO_UPLOAD_MAX_BYTES"); ok {
		parsed, err := strconv.ParseInt(candidate, 10, 64)
		if err != nil || parsed < 1 {
			return errors.New("DOKOSOKO_UPLOAD_MAX_BYTES must be a positive integer")
		}
		value.UploadMaxBytes = parsed
		t.set("uploads.max_bytes", SourceEnvironment, candidate, false, true)
	}
	if err := applyPositiveInt("crawler.max_pages", "DOKOSOKO_CRAWLER_MAX_PAGES", &value.Crawler.MaxPages); err != nil {
		return err
	}
	if err := applyPositiveInt("crawler.max_bytes", "DOKOSOKO_CRAWLER_MAX_BYTES", &value.Crawler.MaxBytes); err != nil {
		return err
	}
	if candidate, ok := nonEmptyEnvironment(t.options.LookupEnv, "DOKOSOKO_AI_MAX_INPUT_TOKENS"); ok {
		parsed, err := strconv.Atoi(candidate)
		if err != nil {
			return errors.New("DOKOSOKO_AI_MAX_INPUT_TOKENS must be an integer")
		}
		value.AI.Analysis.MaxInputTokens = &parsed
		t.set("ai.analysis.max_input_tokens", SourceEnvironment, candidate, false, true)
	}
	if candidate, ok := nonEmptyEnvironment(t.options.LookupEnv, "DOKOSOKO_AI_MAX_OUTPUT_TOKENS"); ok {
		parsed, err := strconv.Atoi(candidate)
		if err != nil {
			return errors.New("DOKOSOKO_AI_MAX_OUTPUT_TOKENS must be an integer")
		}
		value.AI.Analysis.MaxOutputTokens = &parsed
		t.set("ai.analysis.max_output_tokens", SourceEnvironment, candidate, false, true)
	}
	if candidate, ok := nonEmptyEnvironment(t.options.LookupEnv, "DOKOSOKO_AI_DAILY_TOKEN_BUDGET"); ok {
		parsed, err := strconv.ParseInt(candidate, 10, 64)
		if err != nil {
			return errors.New("DOKOSOKO_AI_DAILY_TOKEN_BUDGET must be an integer")
		}
		value.AI.Analysis.DailyTokenBudget = &parsed
		t.set("ai.analysis.daily_token_budget", SourceEnvironment, candidate, false, true)
	}
	applyList := func(key, name string, target *[]string) {
		if candidate, ok := nonEmptyEnvironment(t.options.LookupEnv, name); ok {
			*target = cleanStrings(strings.Split(candidate, ","))
			t.set(key, SourceEnvironment, strings.Join(*target, ","), false, len(*target) > 0)
		}
	}
	applyList("tools.localhost_hosts", "DOKOSOKO_TOOL_LOCALHOST_HOSTS", &value.ToolLocalhostHosts)
	applyList("native_plugins.required", "DOKOSOKO_NATIVE_PLUGINS_REQUIRED", &value.NativePluginsRequired)
	applyList("native_plugins.disabled", "DOKOSOKO_NATIVE_PLUGINS_DISABLED", &value.NativePluginsDisabled)
	if candidate, ok := nonEmptyEnvironment(t.options.LookupEnv, "DOKOSOKO_CRAWLER_LOCALHOST_PORTS"); ok {
		parts := strings.Split(candidate, ",")
		ports := make([]int, 0, len(parts))
		for _, part := range parts {
			port, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				return errors.New("DOKOSOKO_CRAWLER_LOCALHOST_PORTS contains an invalid port")
			}
			ports = append(ports, port)
		}
		value.Crawler.LocalhostPorts = ports
		t.set("crawler.localhost_ports", SourceEnvironment, joinInts(ports), false, len(ports) > 0)
	}
	return nil
}

func (t *tracker) environmentSecret(key, directName, fileName, fallback string) (string, error) {
	direct, directSet := nonEmptyEnvironment(t.options.LookupEnv, directName)
	path, fileSet := nonEmptyEnvironment(t.options.LookupEnv, fileName)
	if directSet && fileSet {
		return "", fmt.Errorf("%s and %s cannot both be set", directName, fileName)
	}
	if directSet {
		t.set(key, SourceEnvironment, "", true, true)
		return direct, nil
	}
	if fileSet {
		absolute, err := absolutePath(path, t.options.WorkingDirectory)
		if err != nil {
			return "", fmt.Errorf("resolve %s: %w", fileName, err)
		}
		contents, err := t.options.ReadFile(absolute)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", fileName, err)
		}
		value := strings.TrimSpace(string(contents))
		t.set(key, SourceEnvironment, "", true, value != "")
		return value, nil
	}
	return fallback, nil
}

func validate(value Config) error {
	if value.Listen == "" {
		return errors.New("server.listen must not be empty")
	}
	if value.PublicURL == "" {
		return errors.New("server.public_url must not be empty")
	}
	if value.UploadMaxBytes < 1 {
		return errors.New("uploads.max_bytes must be a positive integer")
	}
	if value.Crawler.MaxPages < 1 || value.Crawler.MaxBytes < 1 {
		return errors.New("crawler limits must be positive integers")
	}
	if len(value.Crawler.LocalhostPorts) == 0 {
		return errors.New("crawler.localhost_ports must contain at least one port")
	}
	for _, port := range value.Crawler.LocalhostPorts {
		if port < 1 || port > 65535 {
			return errors.New("crawler.localhost_ports contains an invalid port")
		}
	}
	if (value.AI.Provider == "") != (value.AI.APIKey == "") {
		return errors.New("ai.provider and ai.api_key must be configured together")
	}
	if configured := value.AI.Analysis.MaxInputTokens; configured != nil && (*configured < 256 || *configured > 1_000_000) {
		return errors.New("ai.analysis.max_input_tokens is outside supported bounds")
	}
	if configured := value.AI.Analysis.MaxOutputTokens; configured != nil && (*configured < 1 || *configured > 32_768) {
		return errors.New("ai.analysis.max_output_tokens is outside supported bounds")
	}
	if configured := value.AI.Analysis.DailyTokenBudget; configured != nil && (*configured < 0 || *configured > 10_000_000_000) {
		return errors.New("ai.analysis.daily_token_budget is outside supported bounds")
	}
	for key, configured := range map[string]*string{
		"control_plane.organisation.name": value.ControlPlane.Organisation.Name,
		"control_plane.deployment.name":   value.ControlPlane.Deployment.Name,
	} {
		if configured != nil && (len(*configured) < 1 || len(*configured) > 120) {
			return fmt.Errorf("%s must be between 1 and 120 characters", key)
		}
	}
	for key, configured := range map[string]*string{
		"control_plane.organisation.slug": value.ControlPlane.Organisation.Slug,
		"control_plane.deployment.slug":   value.ControlPlane.Deployment.Slug,
	} {
		if configured != nil && (len(*configured) > 63 || !configurationSlugPattern.MatchString(*configured)) {
			return fmt.Errorf("%s must use lower-case letters, numbers, and single hyphens", key)
		}
	}
	if configured := value.ControlPlane.Deployment.Description; configured != nil && len(*configured) > 2000 {
		return errors.New("control_plane.deployment.description must be 2000 characters or fewer")
	}
	if value.ControlPlane.Environments != nil {
		seen := make(map[string]bool)
		productionCount := 0
		for _, environment := range *value.ControlPlane.Environments {
			environment.Name, environment.Slug = strings.TrimSpace(environment.Name), strings.TrimSpace(environment.Slug)
			if len(environment.Name) < 1 || len(environment.Name) > 120 || len(environment.Slug) > 63 || !configurationSlugPattern.MatchString(environment.Slug) {
				return errors.New("control_plane.environments contains an invalid name or slug")
			}
			if seen[environment.Slug] {
				return errors.New("control_plane.environments contains a duplicate slug")
			}
			seen[environment.Slug] = true
			if environment.IsProduction {
				productionCount++
			}
		}
		if productionCount > 1 {
			return errors.New("control_plane.environments can contain at most one production environment")
		}
	}
	return nil
}

var configurationSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (t *tracker) list() []Item {
	result := make([]Item, 0, len(t.order))
	for _, key := range t.order {
		result = append(result, t.items[key])
	}
	return result
}

func environmentValue(lookup func(string) (string, bool), name string) string {
	value, _ := lookup(name)
	return value
}

func nonEmptyEnvironment(lookup func(string) (string, bool), name string) (string, bool) {
	value, ok := lookup(name)
	value = strings.TrimSpace(value)
	return value, ok && value != ""
}

func absolutePath(value, base string) (string, error) {
	if !filepath.IsAbs(value) {
		value = filepath.Join(base, value)
	}
	return filepath.Abs(value)
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func joinInts(values []int) string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strconv.Itoa(value))
	}
	return strings.Join(result, ",")
}

func environmentSummary(values []ControlPlaneEnvironment) string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		label := value.Slug
		if value.IsProduction {
			label += ":production"
		}
		result = append(result, label)
	}
	return strings.Join(result, ",")
}

func parseBoolean(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}
