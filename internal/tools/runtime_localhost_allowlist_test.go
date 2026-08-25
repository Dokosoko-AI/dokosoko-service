package tools

import (
	"context"
	"errors"
	"net"
	"testing"
)

type localhostAllowlistResolver struct{ address net.IP }

func (r localhostAllowlistResolver) LookupIP(context.Context, string, string) ([]net.IP, error) {
	return []net.IP{r.address}, nil
}

func TestSafeDestinationAllowsConfiguredPrivateLocalhost(t *testing.T) {
	runtime := NewRuntime(nil, localhostAllowlistResolver{address: net.ParseIP("192.168.65.2")}, nil)
	runtime.SetPrivateLocalhostHosts([]string{"identity.vendor.localhost:8080", " API.VENDOR.LOCALHOST:80 "})
	if _, _, err := runtime.safeDestination(context.Background(), "http://api.vendor.localhost/status"); err != nil {
		t.Fatalf("configured private .localhost destination was rejected: %v", err)
	}
}

func TestSafeDestinationRejectsUnconfiguredPrivateLocalhost(t *testing.T) {
	runtime := NewRuntime(nil, localhostAllowlistResolver{address: net.ParseIP("192.168.65.2")}, nil)
	runtime.SetPrivateLocalhostHosts([]string{"other.vendor.localhost"})
	if _, _, err := runtime.safeDestination(context.Background(), "http://api.vendor.localhost/status"); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("unconfigured private .localhost destination returned %v", err)
	}
}

func TestSafeDestinationRejectsUnconfiguredLoopbackLocalhost(t *testing.T) {
	runtime := NewRuntime(nil, localhostAllowlistResolver{address: net.ParseIP("127.0.0.1")}, nil)
	for _, raw := range []string{"http://localhost/status", "http://api.vendor.localhost/status"} {
		if _, _, err := runtime.safeDestination(context.Background(), raw); !errors.Is(err, ErrUnsafeDestination) {
			t.Fatalf("unconfigured loopback localhost destination %q returned %v", raw, err)
		}
	}
}

func TestSafeDestinationRequiresExactConfiguredLocalhostPort(t *testing.T) {
	runtime := NewRuntime(nil, localhostAllowlistResolver{address: net.ParseIP("127.0.0.1")}, nil)
	runtime.SetPrivateLocalhostHosts([]string{"api.vendor.localhost:38080"})
	if _, _, err := runtime.safeDestination(context.Background(), "http://api.vendor.localhost:38080/status"); err != nil {
		t.Fatalf("exact configured localhost destination was rejected: %v", err)
	}
	if _, _, err := runtime.safeDestination(context.Background(), "http://api.vendor.localhost:38081/status"); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("unconfigured localhost port returned %v", err)
	}
}
