package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/auth"
	"github.com/dokosoko/dokosoko-service/internal/httpapi"
	"github.com/dokosoko/dokosoko-service/internal/identity"
	"github.com/dokosoko/dokosoko-service/internal/mcpbridge"
	packagegateway "github.com/dokosoko/dokosoko-service/internal/packages"
	"github.com/dokosoko/dokosoko-service/internal/platform"
	providerruntime "github.com/dokosoko/dokosoko-service/internal/providers"
	"github.com/dokosoko/dokosoko-service/internal/secrets"
	"github.com/dokosoko/dokosoko-service/internal/store"
	toolruntime "github.com/dokosoko/dokosoko-service/internal/tools"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	address := env("DOKOSOKO_LISTEN", ":8080")
	baseURL := env("DOKOSOKO_PUBLIC_URL", "http://localhost:8080")
	uiDirectory := env("DOKOSOKO_UI_DIR", "./dist/client")
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

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var persistence store.Store
	var authPersistence auth.Store
	var pool *pgxpool.Pool
	devMemory := boolEnv("DOKOSOKO_DEV_MEMORY")
	if databaseURL := strings.TrimSpace(os.Getenv("DOKOSOKO_DATABASE_URL")); databaseURL != "" {
		pool, err = pgxpool.New(ctx, databaseURL)
		if err != nil {
			return fmt.Errorf("configure database: %w", err)
		}
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			return fmt.Errorf("connect database: %w", err)
		}
		if err := store.Migrate(ctx, pool, env("DOKOSOKO_MIGRATIONS_DIR", "./migrations")); err != nil {
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
	packageGateway := packagegateway.New(persistence, vault, packagegateway.Config{DataDirectory: env("DOKOSOKO_DATA_DIR", "./data")})
	toolProxy := toolruntime.NewRuntime(persistence, vault, nil, nil)
	mcpBridge := mcpbridge.New(persistence, vault, baseURL, nil, nil)
	identityBroker := identity.NewBroker(persistence, vault, baseURL, nil, nil)
	usageReporter := identity.NewHookUsage(persistence, vault)
	toolProxy.SetAuthorizer(identity.NewHookAuthorization(persistence, vault))
	toolProxy.SetMCPExecutor(mcpBridge)
	providerProxy := providerruntime.New(persistence, vault, nil, nil)
	handler := httpapi.NewWithOptions(platform.NewWithVault(persistence, vault), httpapi.Options{
		BaseURL: baseURL, UIDirectory: uiDirectory, Auth: authManager,
		AllowDemoTokens: devMemory && boolEnv("DOKOSOKO_ALLOW_DEMO_TOKENS"), PackageGateway: packageGateway, ToolRuntime: toolProxy, IdentityBroker: identityBroker, UsageReporter: usageReporter, ProviderRuntime: providerProxy, MCPBridge: mcpBridge,
	})
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("DokoSoko listening on %s", address)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
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
	if parsed.Scheme != "https" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" && !boolEnv("DOKOSOKO_ALLOW_INSECURE_HTTP") {
		return errors.New("DOKOSOKO_PUBLIC_URL must use HTTPS outside localhost")
	}
	return nil
}
