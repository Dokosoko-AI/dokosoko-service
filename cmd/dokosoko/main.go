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
	"strings"
	"syscall"
	"time"

	airuntime "github.com/dokosoko/dokosoko-service/internal/ai"
	"github.com/dokosoko/dokosoko-service/internal/auth"
	deploymentconfig "github.com/dokosoko/dokosoko-service/internal/config"
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

const httpWriteTimeout = 2 * time.Minute

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	configuration, err := deploymentconfig.Load()
	if err != nil {
		return err
	}
	address := configuration.Listen
	baseURL := configuration.PublicURL
	uploadDirectory, err := prepareSourceUploadDirectory(configuration.UploadDirectory)
	if err != nil {
		return err
	}
	if err := validatePublicURL(baseURL, configuration.AllowInsecureHTTP); err != nil {
		return err
	}
	masterKey, err := decodeMasterKey(configuration.MasterKey)
	if err != nil {
		return err
	}

	startupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var persistence store.Store
	var authPersistence auth.Store
	var pool *pgxpool.Pool
	if databaseURL := configuration.DatabaseURL; databaseURL != "" {
		pool, err = pgxpool.New(startupCtx, databaseURL)
		if err != nil {
			return fmt.Errorf("configure database: %w", err)
		}
		defer pool.Close()
		if err := pool.Ping(startupCtx); err != nil {
			return fmt.Errorf("connect database: %w", err)
		}
		if err := store.Migrate(startupCtx, pool, configuration.MigrationsDirectory); err != nil {
			return err
		}
		postgres := store.NewPostgres(pool, baseURL)
		persistence, authPersistence = postgres, postgres
	} else if configuration.DevMemory {
		memory := store.NewMemory()
		persistence, authPersistence = memory, memory
		log.Print("WARNING: DOKOSOKO_DEV_MEMORY is enabled; all data will be lost at restart")
	} else {
		return errors.New("DOKOSOKO_DATABASE_URL is required (use DOKOSOKO_DEV_MEMORY=true only for local disposable development)")
	}
	setupComplete, err := authPersistence.SetupCompleted(startupCtx)
	if err != nil {
		return fmt.Errorf("check initial setup status: %w", err)
	}
	setupToken := strings.TrimSpace(configuration.SetupToken)
	if !setupComplete && (setupToken == "" || strings.HasPrefix(setupToken, "replace-")) {
		return errors.New("DOKOSOKO_SETUP_TOKEN must be a strong, one-time random value until initial setup is complete")
	}
	if setupComplete {
		setupToken = ""
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
	configuredEnvironments := make([]platform.ControlPlaneEnvironmentConfiguration, 0)
	if configuration.ControlPlane.Environments != nil {
		configuredEnvironments = make([]platform.ControlPlaneEnvironmentConfiguration, 0, len(*configuration.ControlPlane.Environments))
		for _, environment := range *configuration.ControlPlane.Environments {
			configuredEnvironments = append(configuredEnvironments, platform.ControlPlaneEnvironmentConfiguration{Name: environment.Name, Slug: environment.Slug, IsProduction: environment.IsProduction})
		}
	}
	var environmentConfiguration *[]platform.ControlPlaneEnvironmentConfiguration
	if configuration.ControlPlane.Environments != nil {
		environmentConfiguration = &configuredEnvironments
	}
	if err := platformService.ConfigureControlPlane(startupCtx, platform.ControlPlaneConfiguration{
		Organisation: platform.ControlPlaneIdentityConfiguration{Name: configuration.ControlPlane.Organisation.Name, Slug: configuration.ControlPlane.Organisation.Slug},
		Deployment: platform.ControlPlaneDeploymentConfiguration{
			Name: configuration.ControlPlane.Deployment.Name, Slug: configuration.ControlPlane.Deployment.Slug, Description: configuration.ControlPlane.Deployment.Description,
			FeedbackSubmissionURL: configuration.ControlPlane.Deployment.FeedbackSubmissionURL, ErrorSubmissionURL: configuration.ControlPlane.Deployment.ErrorSubmissionURL,
		},
		Environments: environmentConfiguration,
	}); err != nil {
		return fmt.Errorf("configure control plane: %w", err)
	}
	toolProxy := toolruntime.NewRuntime(persistence, nil, nil)
	toolProxy.SetCredentialResolver(platformService)
	toolProxy.SetPrivateLocalhostHosts(configuration.ToolLocalhostHosts)
	mcpBridge := mcpbridge.New(persistence, vault, nil, nil)
	identityBroker := identity.NewBroker(persistence, vault, baseURL, nil, nil, nil)
	reportingService := reporting.New(persistence)
	toolProxy.SetMCPExecutor(mcpBridge)
	nativePluginManager, err := nativeplugins.New(nativeplugins.Registered(), nativeplugins.Options{
		Environment:           os.LookupEnv,
		Logger:                log.Default(),
		State:                 persistence,
		IdentityKey:           nativePluginIdentityKey(masterKey),
		Required:              configuration.NativePluginsRequired,
		DisabledByEnvironment: configuration.NativePluginsDisabled,
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
		Provider:         configuration.AI.Provider,
		APIKey:           configuration.AI.APIKey,
		Endpoint:         configuration.AI.Endpoint,
		MaxInputTokens:   configuration.AI.Analysis.MaxInputTokens,
		MaxOutputTokens:  configuration.AI.Analysis.MaxOutputTokens,
		DailyTokenBudget: configuration.AI.Analysis.DailyTokenBudget,
		Models: map[airuntime.Workload]string{
			airuntime.WorkloadAnalysis: configuration.AI.Analysis.Model,
		},
	}); err != nil {
		return fmt.Errorf("configure AI: %w", err)
	}
	handler := httpapi.NewWithOptions(platformService, httpapi.Options{
		BaseURL: baseURL, UIDirectory: configuration.UIDirectory, Auth: authManager,
		UploadDirectory: uploadDirectory, UploadMaxBytes: configuration.UploadMaxBytes,
		AllowDemoTokens: configuration.DevMemory && configuration.AllowDemoTokens, ToolRuntime: toolProxy, IdentityBroker: identityBroker, MCPBridge: mcpBridge, NativePlugins: nativePluginManager, Reporting: reportingService, Configuration: configuration.Status,
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
	supervisor.Start("developer-asset-retrieval-retention", func(ctx context.Context) error {
		return platformService.RunDeveloperAssetRetrievalRetentionJanitor(ctx, platform.DefaultDeveloperAssetRetrievalRetentionInterval)
	})
	supervisor.Start("authorization-usage-delivery", func(ctx context.Context) error {
		return toolProxy.RunAuthorizationUsageDelivery(ctx, time.Second)
	})
	supervisor.Start("support-submission-delivery", func(ctx context.Context) error {
		return reportingService.RunDelivery(ctx, time.Second)
	})
	defer func() {
		stopWorkers()
		supervisor.Wait()
	}()
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: httpWriteTimeout, IdleTimeout: 60 * time.Second}
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

func nativePluginIdentityKey(masterKey []byte) []byte {
	mac := hmac.New(sha256.New, masterKey)
	_, _ = mac.Write([]byte("dokosoko-native-plugin-identity-key-v1"))
	return mac.Sum(nil)
}

func prepareSourceUploadDirectory(directory string) (string, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve uploads.directory: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return "", fmt.Errorf("create uploads.directory: %w", err)
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("uploads.directory must be a real directory, not a file or symlink")
	}
	if err := os.Chmod(absolute, 0o700); err != nil {
		return "", fmt.Errorf("secure uploads.directory: %w", err)
	}
	return absolute, nil
}

func decodeMasterKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("DOKOSOKO_MASTER_KEY must be a base64-encoded 32-byte key")
	}
	return decoded, nil
}

func validatePublicURL(value string, allowInsecureHTTP bool) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("DOKOSOKO_PUBLIC_URL must be an absolute http(s) origin without a path, query, or fragment")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return errors.New("DOKOSOKO_PUBLIC_URL must not contain a path")
	}
	if parsed.Scheme != "https" && !identity.IsLocalDevelopmentHostname(parsed.Hostname()) && !allowInsecureHTTP {
		return errors.New("DOKOSOKO_PUBLIC_URL must use HTTPS outside localhost")
	}
	return nil
}
