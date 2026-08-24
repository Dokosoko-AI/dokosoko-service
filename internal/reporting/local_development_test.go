package reporting

import "testing"

func TestDeliveryURLAllowsOnlyRealLocalhostSubdomains(t *testing.T) {
	if !validDeliveryURL("http://api.complicatedauth.localhost:38080/v1/support-submissions") {
		t.Fatal("localhost-subdomain support destination was rejected")
	}
	if validDeliveryURL("http://api.complicatedauth.localhost.example:38080/v1/support-submissions") {
		t.Fatal("lookalike localhost support destination was accepted")
	}
}
