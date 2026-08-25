package store

import (
	"sort"
	"testing"
)

func TestLegacyDuplicateMigrationDependencyOrder(t *testing.T) {
	names := []string{
		"0021_mcp_oauth_clients.sql",
		"0020_package_download_contract.sql",
		"0019_support_routes.sql",
		"0020_contract_v3.sql",
	}
	sort.SliceStable(names, func(i, j int) bool { return migrationNameBefore(names[i], names[j]) })
	want := []string{
		"0019_support_routes.sql",
		"0020_package_download_contract.sql",
		"0020_contract_v3.sql",
		"0021_mcp_oauth_clients.sql",
	}
	for index := range want {
		if names[index] != want[index] {
			t.Fatalf("migration order = %v, want %v", names, want)
		}
	}
}
