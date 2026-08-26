package nativeplugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/netpolicy"
	"github.com/dokosoko/dokosoko-service/nativeplugin"
)

type pluginHost struct {
	config nativeplugin.Config
	logger nativeplugin.Logger
	clock  nativeplugin.Clock
	http   nativeplugin.HTTPClient
}

func (h pluginHost) Config() nativeplugin.Config   { return h.config }
func (h pluginHost) Logger() nativeplugin.Logger   { return h.logger }
func (h pluginHost) Clock() nativeplugin.Clock     { return h.clock }
func (h pluginHost) HTTP() nativeplugin.HTTPClient { return h.http }

func (m *Manager) host(entry *entry) (nativeplugin.Host, error) {
	client, err := newManagedHTTPClient(entry.manifest, entry.config)
	if err != nil {
		return nil, err
	}
	return pluginHost{config: entry.config, logger: pluginLogger{base: m.logger, pluginID: entry.manifest.ID, version: entry.manifest.Version, redactions: configRedactions(entry.manifest, entry.config)}, clock: utcClock{}, http: client}, nil
}

type utcClock struct{}

func (utcClock) Now() time.Time { return time.Now().UTC() }

type pluginLogger struct {
	base              *log.Logger
	pluginID, version string
	redactions        []string
}

func (l pluginLogger) Debug(message string, fields ...nativeplugin.Field) {
	l.write("DEBUG", message, fields)
}
func (l pluginLogger) Info(message string, fields ...nativeplugin.Field) {
	l.write("INFO", message, fields)
}
func (l pluginLogger) Warn(message string, fields ...nativeplugin.Field) {
	l.write("WARN", message, fields)
}
func (l pluginLogger) Error(message string, fields ...nativeplugin.Field) {
	l.write("ERROR", message, fields)
}
func (l pluginLogger) write(level, message string, fields []nativeplugin.Field) {
	parts := []string{"native_plugin=" + l.pluginID, "version=" + l.version, "level=" + level, "message=" + l.scrub(message)}
	for _, field := range fields {
		key := strings.ToLower(strings.TrimSpace(field.Key))
		if key == "" || strings.ContainsAny(key, " \t\r\n=") {
			continue
		}
		value := fmt.Sprint(field.Value)
		if strings.Contains(key, "secret") || strings.Contains(key, "token") || strings.Contains(key, "password") || strings.Contains(key, "credential") {
			value = "[REDACTED]"
		}
		parts = append(parts, key+"="+l.scrub(value))
	}
	l.base.Print(strings.Join(parts, " "))
}

func (l pluginLogger) scrub(value string) string {
	for _, redaction := range l.redactions {
		if redaction != "" {
			value = strings.ReplaceAll(value, redaction, "[REDACTED]")
		}
	}
	return boundedLog(value)
}

func configRedactions(manifest nativeplugin.Manifest, config nativeplugin.Config) []string {
	values := make([]string, 0, len(manifest.Config))
	for _, spec := range manifest.Config {
		if spec.Type != nativeplugin.ConfigSecret {
			continue
		}
		if secret, ok := config.Secret(spec.Key); ok && secret.Reveal() != "" {
			values = append(values, secret.Reveal())
		}
	}
	sort.Slice(values, func(left, right int) bool { return len(values[left]) > len(values[right]) })
	return values
}
func boundedLog(value string) string {
	value = strings.NewReplacer("\r", " ", "\n", " ", "\x00", "").Replace(value)
	if len(value) > 500 {
		return value[:500]
	}
	return value
}

type managedHTTPClient struct {
	allowed map[string]bool
	client  *http.Client
}

func newManagedHTTPClient(manifest nativeplugin.Manifest, config nativeplugin.Config) (*managedHTTPClient, error) {
	allowed := make(map[string]bool, len(manifest.Network))
	for _, claim := range manifest.Network {
		host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(claim.Host), "."))
		if claim.ConfigKey != "" {
			configured, ok := config.URL(claim.ConfigKey)
			if !ok {
				return nil, fmt.Errorf("network URL configuration %s is unavailable", claim.ConfigKey)
			}
			host = strings.ToLower(strings.TrimSuffix(configured.Hostname(), "."))
		}
		allowed[host] = true
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.MaxResponseHeaderBytes = 64 << 10
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil || !allowed[strings.ToLower(strings.TrimSuffix(host, "."))] {
			return nil, errors.New("native plugin network destination is not declared")
		}
		addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil || len(addresses) == 0 {
			return nil, errors.New("native plugin network destination could not be resolved safely")
		}
		for _, address := range addresses {
			if netpolicy.UnsafeIP(address) {
				return nil, errors.New("native plugin network destination resolved to a private or local address")
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(addresses[0].String(), port))
	}
	managed := &managedHTTPClient{allowed: allowed}
	managed.client = &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("native plugin HTTP redirect limit exceeded")
		}
		return managed.validateURL(request.URL)
	}}
	return managed, nil
}

func (c *managedHTTPClient) Do(ctx context.Context, request nativeplugin.HTTPRequest) (nativeplugin.HTTPResponse, error) {
	if len(request.Body) > 1<<20 {
		return nativeplugin.HTTPResponse{}, errors.New("native plugin HTTP request exceeds 1 MiB")
	}
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return nativeplugin.HTTPResponse{}, errors.New("native plugin HTTP method is not supported")
	}
	parsed, err := url.Parse(request.URL)
	if err != nil || parsed == nil {
		return nativeplugin.HTTPResponse{}, errors.New("native plugin HTTP request URL is invalid")
	}
	if err := c.validateURL(parsed); err != nil {
		return nativeplugin.HTTPResponse{}, err
	}
	outbound, err := http.NewRequestWithContext(ctx, method, parsed.String(), bytes.NewReader(request.Body))
	if err != nil {
		return nativeplugin.HTTPResponse{}, errors.New("native plugin HTTP request is invalid")
	}
	for key, value := range request.Headers {
		if !allowedPluginHeader(key) {
			continue
		}
		outbound.Header.Set(key, value)
	}
	outbound.Header.Set("User-Agent", "DokoSoko-Native-Plugin/1")
	response, err := c.client.Do(outbound)
	if err != nil {
		return nativeplugin.HTTPResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(&limitedBody{reader: response.Body, closer: response.Body, remaining: 1 << 20, message: "native plugin HTTP response exceeds 1 MiB"})
	if err != nil {
		return nativeplugin.HTTPResponse{}, err
	}
	return nativeplugin.HTTPResponse{StatusCode: response.StatusCode, Headers: response.Header.Clone(), Body: body}, nil
}

func allowedPluginHeader(key string) bool {
	key = http.CanonicalHeaderKey(strings.TrimSpace(key))
	switch key {
	case "", "Connection", "Host", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade", "User-Agent":
		return false
	default:
		return true
	}
}

func (c *managedHTTPClient) validateURL(value *url.URL) error {
	if value.Scheme != "https" || value.Host == "" || value.User != nil || value.Port() != "" && value.Port() != "443" || !c.allowed[strings.ToLower(strings.TrimSuffix(value.Hostname(), "."))] {
		return errors.New("native plugin HTTPS destination is not declared")
	}
	return nil
}

type limitedBody struct {
	reader    io.Reader
	closer    io.Closer
	remaining int64
	message   string
}

func (b *limitedBody) Read(buffer []byte) (int, error) {
	if b.remaining == 0 {
		var probe [1]byte
		n, err := b.reader.Read(probe[:])
		if n > 0 {
			return 0, errors.New(b.message)
		}
		return 0, err
	}
	if int64(len(buffer)) > b.remaining {
		buffer = buffer[:b.remaining]
	}
	n, err := b.reader.Read(buffer)
	b.remaining -= int64(n)
	return n, err
}
func (b *limitedBody) Close() error { return b.closer.Close() }
