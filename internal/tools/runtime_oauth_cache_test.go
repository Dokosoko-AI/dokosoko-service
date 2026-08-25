package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

type oauthCacheResolver struct{ address net.IP }

func (resolver oauthCacheResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{resolver.address}, nil
}

type oauthCacheDoer func(*http.Request) (*http.Response, error)

func (do oauthCacheDoer) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

type oauthCacheCredentialResolver struct {
	mu         sync.Mutex
	credential []byte
	calls      int
}

func (resolver *oauthCacheCredentialResolver) ResolveToolCredential(context.Context, model.Tool) ([]byte, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	resolver.calls++
	return resolver.credential, nil
}

func (resolver *oauthCacheCredentialResolver) callCount() int {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	return resolver.calls
}

func TestOAuthClientTokenCoalescesConcurrentExchangesAndWaitersCanCancel(t *testing.T) {
	credential := []byte("client-secret")
	credentials := &oauthCacheCredentialResolver{credential: credential}
	exchangeStarted := make(chan struct{})
	releaseExchange := make(chan struct{})
	var startOnce sync.Once
	var tokenCalls atomic.Int32
	runtime := NewRuntime(nil, oauthCacheResolver{address: net.ParseIP("8.8.8.8")}, oauthCacheDoer(func(*http.Request) (*http.Response, error) {
		tokenCalls.Add(1)
		startOnce.Do(func() { close(exchangeStarted) })
		<-releaseExchange
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"coalesced-token","token_type":"Bearer","expires_in":300}`)),
		}, nil
	}))
	runtime.SetCredentialResolver(credentials)
	runtime.now = func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) }
	tool := model.Tool{APIConnectionID: "connection-1", CredentialID: "credential-1", TimeoutMS: 5000}
	auth := upstreamAuth{Type: "oauth_client_credentials", ClientID: "client-1", TokenURL: "https://identity.example.test/oauth/token"}

	type result struct {
		tokenType   string
		accessToken string
		err         error
	}
	leaderResult := make(chan result, 1)
	go func() {
		tokenType, accessToken, err := runtime.oauthClientToken(context.Background(), tool, auth)
		leaderResult <- result{tokenType: tokenType, accessToken: accessToken, err: err}
	}()
	<-exchangeStarted

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := runtime.oauthClientToken(canceledContext, tool, auth); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v", err)
	}
	if calls := tokenCalls.Load(); calls != 1 {
		t.Fatalf("canceled waiter started %d token exchanges", calls)
	}

	const waiterCount = 32
	startWaiters := make(chan struct{})
	waiterResults := make(chan result, waiterCount)
	var waitersReady sync.WaitGroup
	waitersReady.Add(waiterCount)
	for range waiterCount {
		go func() {
			waitersReady.Done()
			<-startWaiters
			tokenType, accessToken, err := runtime.oauthClientToken(context.Background(), tool, auth)
			waiterResults <- result{tokenType: tokenType, accessToken: accessToken, err: err}
		}()
	}
	waitersReady.Wait()
	close(startWaiters)

	// Keep the leader in flight long enough for the released goroutines to join
	// it. Any implementation without per-key coalescing starts more exchanges.
	time.Sleep(25 * time.Millisecond)
	if calls := tokenCalls.Load(); calls != 1 {
		close(releaseExchange)
		t.Fatalf("concurrent waiters started %d token exchanges", calls)
	}
	close(releaseExchange)

	leader := <-leaderResult
	if leader.err != nil || leader.tokenType != "Bearer" || leader.accessToken != "coalesced-token" {
		t.Fatalf("leader result = %#v", leader)
	}
	for range waiterCount {
		waiter := <-waiterResults
		if waiter.err != nil || waiter.tokenType != "Bearer" || waiter.accessToken != "coalesced-token" {
			t.Fatalf("waiter result = %#v", waiter)
		}
	}
	if calls := tokenCalls.Load(); calls != 1 {
		t.Fatalf("token exchanges = %d", calls)
	}
	if calls := credentials.callCount(); calls != 1 {
		t.Fatalf("credential resolutions = %d", calls)
	}
	for index, value := range credential {
		if value != 0 {
			t.Fatalf("credential byte %d was not wiped", index)
		}
	}
}

func TestOAuthTokenCachePurgesAndWipesTokensPastRefreshBoundary(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	runtime := NewRuntime(nil, nil, nil)
	expiredToken := []byte("expired-token")
	usableToken := []byte("usable-token")
	runtime.tokens = map[string]cachedOAuthToken{
		"expired": {AccessToken: expiredToken, TokenType: "Bearer", ExpiresAt: now.Add(oauthTokenRefreshSkew)},
		"usable":  {AccessToken: usableToken, TokenType: "Bearer", ExpiresAt: now.Add(oauthTokenRefreshSkew + time.Nanosecond)},
	}

	runtime.tokenMu.Lock()
	runtime.purgeExpiredOAuthTokensLocked(now)
	runtime.tokenMu.Unlock()

	if _, ok := runtime.tokens["expired"]; ok {
		t.Fatal("expired token remained cached")
	}
	for index, value := range expiredToken {
		if value != 0 {
			t.Fatalf("expired token byte %d was not wiped", index)
		}
	}
	if cached, ok := runtime.tokens["usable"]; !ok || string(cached.AccessToken) != "usable-token" {
		t.Fatalf("usable token was purged: %#v", cached)
	}
}

func TestOAuthTokenCacheIsBoundedAndWipesEvictedOrReplacedTokens(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	runtime := NewRuntime(nil, nil, nil)
	runtime.tokenMu.Lock()
	for index := range maxOAuthTokenCacheEntries {
		key := fmt.Sprintf("cache-%03d", index)
		runtime.cacheOAuthTokenLocked(key, "Bearer", fmt.Sprintf("token-%03d", index), now.Add(time.Duration(index+1)*time.Minute))
	}
	evictedBytes := runtime.tokens["cache-000"].AccessToken
	runtime.cacheOAuthTokenLocked("cache-new", "Bearer", "new-token", now.Add(24*time.Hour))
	if size := len(runtime.tokens); size != maxOAuthTokenCacheEntries {
		runtime.tokenMu.Unlock()
		t.Fatalf("cache size = %d", size)
	}
	if _, ok := runtime.tokens["cache-000"]; ok {
		runtime.tokenMu.Unlock()
		t.Fatal("earliest-expiring token was not evicted")
	}
	for index, value := range evictedBytes {
		if value != 0 {
			runtime.tokenMu.Unlock()
			t.Fatalf("evicted token byte %d was not wiped", index)
		}
	}

	replacedBytes := runtime.tokens["cache-001"].AccessToken
	runtime.cacheOAuthTokenLocked("cache-001", "Bearer", "replacement-token", now.Add(48*time.Hour))
	replacement := string(runtime.tokens["cache-001"].AccessToken)
	size := len(runtime.tokens)
	runtime.tokenMu.Unlock()

	if size != maxOAuthTokenCacheEntries || replacement != "replacement-token" {
		t.Fatalf("replacement cache size/token = %d/%q", size, replacement)
	}
	for index, value := range replacedBytes {
		if value != 0 {
			t.Fatalf("replaced token byte %d was not wiped", index)
		}
	}
}
