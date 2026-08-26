package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/lifecycle"
	"github.com/dokosoko/dokosoko-service/internal/mcpbridge"
	"github.com/dokosoko/dokosoko-service/internal/nativeplugins"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	"github.com/dokosoko/dokosoko-service/internal/reporting"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	address := env("DOKOSOKO_LISTEN", ":8080")
	baseURL := env("DOKOSOKO_PUBLIC_URL", "http://localhost:8080")
	uiDirectory := env("DOKOSOKO_UI_DIR", "./dist/client")
	uploadDirectory, uploadMaxBytes, err := sourceUploadConfig()
	if err != nil {
		return err
	}
	if err := validatePublicURL(baseURL); err != nil {
		return err
	}
	setupToken := strings.TrimSpace(os.Getenv("DOKOSOKO_SETUP_TOKEN"))
	if setupToken == "" || strings.HasPrefix(setupToken, "replace-") {
		return errors.New("DOKOSOKO_SETUP_TOKEN must be a strong, one-time random value")
	}
	masterKey, err := decodeMasterKey(os.Getenv("DOKOSOKO_MASTER_KEY"))
	if err != nil {
		return err
	}

	startupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var persistence store.Store
	var authPersistence auth.Store
	var pool *pgxpool.Pool
	devMemory := boolEnv("DOKOSOKO_DEV_MEMORY")
	if databaseURL := strings.TrimSpace(os.Getenv("DOKOSOKO_DATABASE_URL")); databaseURL != "" {
		pool, err = pgxpool.New(startupCtx, databaseURL)
		if err != nil {
			return fmt.Errorf("configure database: %w", err)
		}
		defer pool.Close()
		if err := pool.Ping(startupCtx); err != nil {
			return fmt.Errorf("connect database: %w", err)
		}
		if err := store.Migrate(startupCtx, pool, env("DOKOSOKO_MIGRATIONS_DIR", "./migrations")); err != nil {
			return err
		}
		postgres := store.NewPostgres(pool, baseURL)
		persistence, authPersistence = postgres, postgres
	} else if devMemory {
		memory := store.NewMemory()
		persistence, authPersistence = memory, memory
		log.Print("WARNING: DOKOSOKO_DEV_MEMORY is enabled; all data will be lost at restart")
	} else {
		return errors.New("DOKOSOKO_DATABASE_URL is required (use DOKOSOKO_DEV_MEMORY=true only for local disposable development)")
	}

	authManager, err := auth.New(authPersistence, auth.Config{SetupToken: setupToken, MasterKey: masterKey, PublicURL: baseURL, SessionTTL: 8 * time.Hour})
	if err != nil {
		return fmt.Errorf("configure authentication: %w", err)
	}
	vault, err := secrets.New(masterKey)
	if err != nil {
		return fmt.Errorf("configure secret vault: %w", err)
	}
	platformService := platform.NewWithVault(persistence, vault)
	toolProxy := toolruntime.NewRuntime(persistence, nil, nil)
	toolProxy.SetCredentialResolver(platformService)
	toolProxy.SetPrivateLocalhostHosts(strings.Split(os.Getenv("DOKOSOKO_TOOL_LOCALHOST_HOSTS"), ","))
	mcpBridge := mcpbridge.New(persistence, vault, nil, nil)
	identityBroker := identity.NewBroker(persistence, vault, baseURL, nil, nil, nil)
	reportingService := reporting.New(persistence)
	toolProxy.SetMCPExecutor(mcpBridge)
	nativePluginManager, err := nativeplugins.New(nativeplugins.Registered(), nativeplugins.Options{
		Environment:           os.LookupEnv,
		Logger:                log.Default(),
		State:                 persistence,
		IdentityKey:           nativePluginIdentityKey(masterKey),
		Required:              commaSeparatedEnv("DOKOSOKO_NATIVE_PLUGINS_REQUIRED"),
		DisabledByEnvironment: commaSeparatedEnv("DOKOSOKO_NATIVE_PLUGINS_DISABLED"),
	})
	if err != nil {
		return fmt.Errorf("configure native plugins: %w", err)
	}
	if err := nativePluginManager.Start(startupCtx); err != nil {
		return fmt.Errorf("start native plugins: %w", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer closeCancel()
		if err := nativePluginManager.Close(closeCtx); err != nil {
			log.Printf("native plugin shutdown failed: %v", err)
		}
	}()
	toolProxy.SetNativeExecutor(nativePluginManager)
	platformService.SetNativeToolCatalog(nativePluginManager)
	if deployment, deploymentErr := persistence.Deployment(startupCtx); deploymentErr == nil {
		if err := nativePluginManager.SyncCatalog(startupCtx, persistence, deployment); err != nil {
			return fmt.Errorf("synchronize native tool catalog: %w", err)
		}
	} else if !errors.Is(deploymentErr, store.ErrNotFound) {
		return fmt.Errorf("load deployment for native tool catalog: %w", deploymentErr)
	}
	if err := platformService.ConfigureEnvironmentAI(startupCtx, platform.AIEnvironmentConfig{
		Provider: os.Getenv("DOKOSOKO_AI_PROVIDER"),
		APIKey:   os.Getenv("DOKOSOKO_AI_API_KEY"),
		Endpoint: os.Getenv("DOKOSOKO_AI_ENDPOINT"),
		Models: map[airuntime.Workload]string{
			airuntime.WorkloadAnalysis:  os.Getenv("DOKOSOKO_AI_MODEL_ANALYSIS"),
			airuntime.WorkloadAssistant: os.Getenv("DOKOSOKO_AI_MODEL_ASSISTANT"),
		},
	}); err != nil {
		return fmt.Errorf("configure AI: %w", err)
	}
	handler := httpapi.NewWithOptions(platformService, httpapi.Options{
		BaseURL: baseURL, UIDirectory: uiDirectory, Auth: authManager,
		UploadDirectory: uploadDirectory, UploadMaxBytes: uploadMaxBytes,
		AllowDemoTokens: devMemory && boolEnv("DOKOSOKO_ALLOW_DEMO_TOKENS"), ToolRuntime: toolProxy, IdentityBroker: identityBroker, MCPBridge: mcpBridge, NativePlugins: nativePluginManager, Reporting: reportingService,
	})
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", address, err)
	}
	workerCtx, stopWorkers := context.WithCancel(ctx)
	supervisor := lifecycle.NewSupervisor(workerCtx, log.Printf)
	supervisor.Start("tool-test-retention", func(ctx context.Context) error {
		return platformService.RunToolTestRetentionJanitor(ctx, platform.DefaultToolTestRetentionInterval)
	})
	supervisor.Start("identity-oauth-retention", func(ctx context.Context) error {
		return platformService.RunIdentityOAuthRetentionJanitor(ctx, platform.DefaultIdentityOAuthRetentionInterval)
	})
	defer func() {
		stopWorkers()
		supervisor.Wait()
	}()
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("DokoSoko listening on %s", listener.Addr())
	return serve(ctx, server, listener)
}

func serve(ctx context.Context, server *http.Server, listener net.Listener) error {
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	select {
	case err := <-serveErrors:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		if shutdownErr != nil {
			_ = server.Close()
		}
		serveErr := <-serveErrors
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP during shutdown: %w", serveErr)
		}
		if shutdownErr != nil {
			return fmt.Errorf("shut down HTTP server: %w", shutdownErr)
		}
		return nil
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func boolEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func commaSeparatedEnv(key string) []string {
	values := strings.Split(os.Getenv(key), ",")
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func nativePluginIdentityKey(masterKey []byte) []byte {
	mac := hmac.New(sha256.New, masterKey)
	_, _ = mac.Write([]byte("dokosoko-native-plugin-identity-key-v1"))
	return mac.Sum(nil)
}

func sourceUploadConfig() (string, int64, error) {
	const defaultMaxBytes = int64(5_000_000)
	directory := strings.TrimSpace(os.Getenv("DOKOSOKO_UPLOAD_DIR"))
	maxBytes := defaultMaxBytes
	if configured := strings.TrimSpace(os.Getenv("DOKOSOKO_UPLOAD_MAX_BYTES")); configured != "" {
		value, err := strconv.ParseInt(configured, 10, 64)
		if err != nil || value < 1 {
			return "", 0, errors.New("DOKOSOKO_UPLOAD_MAX_BYTES must be a positive integer")
		}
		maxBytes = value
	}
	if directory == "" {
		return "", maxBytes, nil
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", 0, fmt.Errorf("resolve DOKOSOKO_UPLOAD_DIR: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return "", 0, fmt.Errorf("create DOKOSOKO_UPLOAD_DIR: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", 0, errors.New("DOKOSOKO_UPLOAD_DIR must be a real directory, not a file or symlink")
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return "", 0, fmt.Errorf("secure DOKOSOKO_UPLOAD_DIR: %w", err)
	}
	return absolute, maxBytes, nil
}

func decodeMasterKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("DOKOSOKO_MASTER_KEY must be a base64-encoded 32-byte key")
	}
	return decoded, nil
}

func validatePublicURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("DOKOSOKO_PUBLIC_URL must be an absolute http(s) origin without a path, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return errors.New("DOKOSOKO_PUBLIC_URL must not contain a path")
	}
	if parsed.Scheme != "https" && !identity.IsLocalDevelopmentHostname(parsed.Hostname()) && !boolEnv("DOKOSOKO_ALLOW_INSECURE_HTTP") {
		return errors.New("DOKOSOKO_PUBLIC_URL must use HTTPS outside localhost")
	}
	return nil
}
