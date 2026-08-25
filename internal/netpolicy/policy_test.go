package netpolicy

import (
	"net"
	"testing"
)

func TestUnsafeIP(t *testing.T) {
	for _, raw := range []string{
		"127.0.0.1", "10.0.0.1", "100.64.0.1", "192.0.2.1", "224.0.0.1",
		"64:ff9b::c000:201", "64:ff9b:1::c000:201", "fec0::1", "2002:c000:0201::1", "2001::1", "::192.0.2.1",
	} {
		if !UnsafeIP(net.ParseIP(raw)) {
			t.Fatalf("special address %s was accepted", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if UnsafeIP(net.ParseIP(raw)) {
			t.Fatalf("public address %s was rejected", raw)
		}
	}
}
