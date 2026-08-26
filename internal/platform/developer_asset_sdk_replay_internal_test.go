package platform

import (
	"reflect"
	"testing"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

func TestBuildSDKContentCandidateHasIndependentReplayStableUUIDv5Graph(t *testing.T) {
	t.Parallel()
	packageValue := model.SDKPackage{
		ID: "11111111-1111-4111-8111-111111111111", Name: "Replay SDK", CanonicalCoordinate: "example.com/replay/sdk",
	}
	release := model.SDKRelease{
		ID: "22222222-2222-4222-8222-222222222222", SDKPackageID: packageValue.ID, ExactVersion: "v4.5.6",
		InstallCommand: "go get example.com/replay/sdk@v4.5.6", ReleaseHash: "sha256:release", Visibility: model.VisibilityPrivate,
	}
	files := []SDKIngestionFile{
		{SourcePath: "README.md", Content: "# Quickstart\n\n```go\nclient := sdk.NewClient()\n```\n", LicenseExpression: "MIT"},
		{SourcePath: "client.go", Content: "package sdk\n\ntype Client struct{}\n\nfunc NewClient() *Client { return &Client{} }\n", LicenseExpression: "MIT"},
		{SourcePath: "examples/main.go", Content: "package main\n\nfunc main() {}\n", LicenseExpression: "MIT"},
	}
	input := SDKContentIngestionInput{ResolvedSourceURI: "https://example.com/replay/sdk", ResolvedSourceRevision: "commit-replay", Files: files}
	first, err := buildSDKContentCandidate("33333333-3333-4333-8333-333333333333", packageValue, release, input)
	if err != nil {
		t.Fatal(err)
	}
	// A second independent build receives a fresh input graph and a different
	// source order, as separate workers commonly do.
	reversed := []SDKIngestionFile{files[2], files[1], files[0]}
	second, err := buildSDKContentCandidate("33333333-3333-4333-8333-333333333333", packageValue, release, SDKContentIngestionInput{
		ResolvedSourceURI: input.ResolvedSourceURI, ResolvedSourceRevision: input.ResolvedSourceRevision, Files: reversed,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("independent exact-release builds differ:\nfirst=%#v\nsecond=%#v", first, second)
	}

	ids := []string{first.record.Candidate.ID}
	for _, file := range first.record.Files {
		ids = append(ids, file.ID)
	}
	for _, section := range first.record.Sections {
		ids = append(ids, section.ID)
	}
	for _, symbol := range first.record.Symbols {
		ids = append(ids, symbol.ID)
	}
	for _, sample := range first.record.Samples {
		ids = append(ids, sample.ID)
	}
	if first.record.Map == nil {
		t.Fatal("SDK map was not built")
	}
	ids = append(ids, first.record.Map.ID)
	for _, id := range ids {
		if len(id) != 36 || id[14] != '5' {
			t.Fatalf("identity %q is not a UUIDv5", id)
		}
	}

	changedInput := input
	changedInput.Files = append([]SDKIngestionFile(nil), files...)
	changedInput.Files[1].Content += "\nfunc Changed() {}\n"
	changed, err := buildSDKContentCandidate("33333333-3333-4333-8333-333333333333", packageValue, release, changedInput)
	if err != nil {
		t.Fatal(err)
	}
	if changed.record.Candidate.ID == first.record.Candidate.ID {
		t.Fatal("candidate UUID did not change with the exact source manifest")
	}
}
