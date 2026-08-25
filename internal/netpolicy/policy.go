// Package netpolicy defines the service-wide outbound network boundary.
package netpolicy

import "net"

var disallowedNetworks = []*net.IPNet{
	mustCIDR("0.0.0.0/8"),
	mustCIDR("100.64.0.0/10"),
	mustCIDR("192.0.0.0/24"),
	mustCIDR("192.0.2.0/24"),
	mustCIDR("198.18.0.0/15"),
	mustCIDR("198.51.100.0/24"),
	mustCIDR("203.0.113.0/24"),
	mustCIDR("224.0.0.0/4"),
	mustCIDR("240.0.0.0/4"),
	mustCIDR("::/96"),
	mustCIDR("64:ff9b::/96"),
	mustCIDR("64:ff9b:1::/48"),
	mustCIDR("100::/64"),
	mustCIDR("2001::/32"),
	mustCIDR("2001:2::/48"),
	mustCIDR("2001:10::/28"),
	mustCIDR("2001:20::/28"),
	mustCIDR("2001:db8::/32"),
	mustCIDR("2002::/16"),
	mustCIDR("fc00::/7"),
	mustCIDR("fec0::/10"),
	mustCIDR("fe80::/10"),
}

func mustCIDR(raw string) *net.IPNet {
	_, network, err := net.ParseCIDR(raw)
	if err != nil {
		panic("invalid outbound network policy: " + raw)
	}
	return network
}

// UnsafeIP reports whether an address must be rejected for a public outbound
// connection. It intentionally rejects documentation, benchmarking, transition,
// local-use, multicast, and otherwise non-global ranges to prevent SSRF and DNS
// rebinding from reaching infrastructure-only destinations.
func UnsafeIP(address net.IP) bool {
	if address == nil || address.IsUnspecified() || address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() {
		return true
	}
	for _, network := range disallowedNetworks {
		if network.Contains(address) {
			return true
		}
	}
	return false
}

// LocalDevelopmentIP reports whether an address remains inside the explicitly
// supported localhost/private development boundary.
func LocalDevelopmentIP(address net.IP) bool {
	return address != nil && (address.IsLoopback() || address.IsPrivate())
}
