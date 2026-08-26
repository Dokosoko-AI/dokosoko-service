package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validDeveloperAssetHash = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestAPISDKBindingJSONUsesPublicStateField(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(APISDKBinding{State: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	if body["state"] != "ready" {
		t.Fatalf("state = %#v, want ready", body["state"])
	}
	if _, exists := body["binding_state"]; exists {
		t.Fatal("internal database column name leaked into the public JSON contract")
	}
}

func TestCrawlerPersistenceMapFixturesDecodeIntoGoModels(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "crawler", "fixtures", "persistence-map-bodies.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Documentation json.RawMessage `json:"documentation"`
		Contract      json.RawMessage `json:"contract"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	var documentation DocumentationMapBody
	if err := json.Unmarshal(fixture.Documentation, &documentation); err != nil {
		t.Fatalf("crawler documentation map does not satisfy the Go persistence contract: %v", err)
	}
	if documentation.Overview == "" || len(documentation.Documents) != 1 ||
		len(documentation.Topics) == 0 || len(documentation.Authentication) != 1 {
		t.Fatalf("decoded documentation map lost typed TOC evidence: %#v", documentation)
	}
	var contract ContractMapBody
	if err := json.Unmarshal(fixture.Contract, &contract); err != nil {
		t.Fatalf("crawler contract map does not satisfy the Go persistence contract: %v", err)
	}
	if contract.Overview == "" || len(contract.Capabilities) != 1 ||
		len(contract.Operations) != 1 || len(contract.Schemas) != 1 ||
		contract.Operations[0].ID == "" {
		t.Fatalf("decoded contract map lost typed operation evidence: %#v", contract)
	}
}

func TestCanonicalSDKCoordinateMatchesEcosystemRules(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		ecosystem  string
		coordinate string
		want       string
	}{
		{name: "pypi normalizes case and separators", ecosystem: "PyPI", coordinate: "  Acme_SDK.Extra  ", want: "acme-sdk-extra"},
		{name: "npm normalizes case", ecosystem: "npm", coordinate: " @Acme/Client ", want: "@acme/client"},
		{name: "cargo normalizes case", ecosystem: "cargo", coordinate: "Acme_Client", want: "acme_client"},
		{name: "go preserves module path case", ecosystem: "go", coordinate: " Example.com/Acme/SDK ", want: "Example.com/Acme/SDK"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := CanonicalSDKCoordinate(test.ecosystem, test.coordinate); got != test.want {
				t.Fatalf("CanonicalSDKCoordinate(%q, %q) = %q, want %q", test.ecosystem, test.coordinate, got, test.want)
			}
		})
	}
}

func TestSDKReleaseRequiresExactImmutableIdentity(t *testing.T) {
	t.Parallel()
	valid := SDKRelease{
		ExactVersion:      "2.1.0",
		InstallCommand:    "npm install @acme/sdk@2.1.0",
		IdentityAssurance: "metadata_only",
		Visibility:        VisibilityPrivate,
		ReleaseHash:       validDeveloperAssetHash,
	}
	if !valid.Valid() {
		t.Fatal("valid exact release was rejected")
	}
	for _, version := range []string{"", "latest", "LATEST", "^2.1.0", ">=2.0.0", "2.*"} {
		candidate := valid
		candidate.ExactVersion = version
		if candidate.Valid() {
			t.Fatalf("non-exact version %q was accepted", version)
		}
	}
	verified := valid
	verified.IdentityAssurance = "verified_digest"
	if verified.Valid() {
		t.Fatal("verified release without a digest was accepted")
	}
	verified.UpstreamDigest = "sha256:" + strings.Repeat("b", 64)
	if !verified.Valid() {
		t.Fatal("verified release with a valid digest was rejected")
	}
}

func TestDraftBindingsResolveExactlyAtPublicationBoundary(t *testing.T) {
	t.Parallel()
	for _, binding := range []APIDocumentationBinding{
		{FollowLatest: true, Visibility: VisibilityPrivate},
		{FollowLatest: false, PinnedRevisionID: "revision", Visibility: VisibilityPublic},
	} {
		if !binding.Valid() {
			t.Fatalf("valid binding was rejected: %#v", binding)
		}
	}
	for _, binding := range []APIDocumentationBinding{
		{FollowLatest: true, PinnedRevisionID: "revision", Visibility: VisibilityPrivate},
		{FollowLatest: false, Visibility: VisibilityPrivate},
	} {
		if binding.Valid() {
			t.Fatalf("ambiguous binding was accepted: %#v", binding)
		}
	}
}

func TestAPISDKBindingSeparatesApplicabilityFromCompatibility(t *testing.T) {
	t.Parallel()
	valid := APISDKBinding{
		APIID:        "api",
		SDKPackageID: "package",
		SDKReleaseID: "release",
		State:        "legacy_metadata",
		Coverage:     SDKCoverageUnknown,
		Assurance:    SDKAssuranceRelated,
		SelectorHash: validDeveloperAssetHash,
		Visibility:   VisibilityPrivate,
	}
	if !valid.Valid() {
		t.Fatal("legacy exact SDK binding was rejected")
	}
	ready := valid
	ready.State = "ready"
	if ready.Valid() {
		t.Fatal("ready binding without an exact content publication was accepted")
	}
	ready.SDKContentPublicationID = "content-publication"
	ready.Assurance = SDKAssuranceVerified
	if ready.Valid() {
		t.Fatal("verified binding without a reviewed compatibility assertion was accepted")
	}
	ready.CompatibilityAssertionID = "assertion"
	if !ready.Valid() {
		t.Fatal("fully pinned verified SDK binding was rejected")
	}
}

func TestSDKSampleCandidateCanBeReviewedBeforeImmutablePublication(t *testing.T) {
	t.Parallel()
	valid := SDKCodeSample{
		Language:           "typescript",
		Title:              "Create a customer",
		Intent:             "Create one customer with the API client",
		Code:               "await client.customers.create({name: 'Ada'})",
		Origin:             SDKSampleCurated,
		ValidationStatus:   SDKSampleSyntaxChecked,
		ValidationEvidence: json.RawMessage(`{"validated":true,"validator":"test/parser"}`),
		Visibility:         VisibilityPrivate,
		ContentHash:        validDeveloperAssetHash,
	}
	if !valid.Valid() {
		t.Fatal("reviewed and syntax-checked sample was rejected")
	}
	unvalidated := valid
	unvalidated.ValidationStatus = SDKSampleUnvalidated
	if !unvalidated.Valid() {
		t.Fatal("unvalidated candidate must remain inspectable before review")
	}
	ordinal := 0
	selection := SDKContentPublicationSampleSelection{
		SDKContentCandidateID: valid.SDKContentCandidateID,
		SDKCodeSampleID:       valid.ID,
		Decision:              "approved",
		Ordinal:               &ordinal,
		ContentHash:           valid.ContentHash,
	}
	if selection.ValidFor(unvalidated) {
		t.Fatal("unvalidated candidate was approved for immutable publication")
	}
	if !selection.ValidFor(valid) {
		t.Fatal("validated candidate could not be approved for immutable publication")
	}
	statusOnly := valid
	statusOnly.ValidationEvidence = json.RawMessage(`{}`)
	if statusOnly.HasPositiveMachineValidationEvidence() || selection.ValidFor(statusOnly) {
		t.Fatal("syntax_checked status without positive parser evidence became approvable")
	}
	unnamedValidator := valid
	unnamedValidator.ValidationEvidence = json.RawMessage(`{"validated":true}`)
	if unnamedValidator.HasPositiveMachineValidationEvidence() || selection.ValidFor(unnamedValidator) {
		t.Fatal("validation result without a named validator or evidence ID became approvable")
	}
	selection.ReviewEvidence = json.RawMessage(`{"summary":"Reviewer parsed the exact candidate with an independently pinned grammar parser."}`)
	if !selection.ValidFor(statusOnly) {
		t.Fatal("explicit structured review evidence did not enable conservative manual approval")
	}
	extracted := valid
	extracted.Origin = SDKSampleExtracted
	if extracted.Valid() {
		t.Fatal("extracted sample without exact source lineage was accepted")
	}
	extracted.SourcePath = "examples/customers.ts"
	extracted.SourceRevision = "v2.1.0"
	if !extracted.Valid() {
		t.Fatal("extracted sample with exact source lineage was rejected")
	}
}

func TestIngestionRunRequiresDeterministicVersionsAndLeaseShape(t *testing.T) {
	t.Parallel()
	valid := DeveloperAssetIngestionRun{
		AssetKind: DeveloperAssetDocumentation,
		TargetID:  "source",
		TargetKey: "source:docs",
		SourceID:  "source",
		State:     DeveloperAssetIngestionQueued,
		Attempt:   1,
		Versions: ProcessorVersions{
			Pipeline: "pipeline-v1", Parser: "html-v1", Normalizer: "docs-v1", Mapper: "map-v1",
		},
	}
	if !valid.Valid() {
		t.Fatal("valid deterministic ingestion run was rejected")
	}
	missingParser := valid
	missingParser.Versions.Parser = ""
	if missingParser.Valid() {
		t.Fatal("ingestion without a parser version was accepted")
	}
	leased := valid
	leased.LeaseOwner = "worker-1"
	if leased.Valid() {
		t.Fatal("lease owner without expiry was accepted")
	}
	expires := time.Now().Add(time.Minute)
	leased.LeaseExpiresAt = &expires
	if !leased.Valid() {
		t.Fatal("complete worker lease was rejected")
	}
	if !DeveloperAssetIngestionQueued.CanTransitionTo(DeveloperAssetIngestionRunning) ||
		DeveloperAssetIngestionPublished.CanTransitionTo(DeveloperAssetIngestionRunning) {
		t.Fatal("ingestion state machine accepted an invalid transition")
	}
}

func TestAPIContractSourceHasOneTypedTarget(t *testing.T) {
	t.Parallel()
	valid := APIContractSource{
		DeploymentID:  "deployment",
		APIContractID: "contract",
		SourceID:      "source",
		SourceRole:    "primary",
		Lifecycle:     "attached",
		Revision:      1,
	}
	if !valid.Valid() {
		t.Fatal("valid contract source association was rejected")
	}
	invalid := valid
	invalid.SourceRole = "ambiguous"
	if invalid.Valid() {
		t.Fatal("unknown contract source role was accepted")
	}
}

func TestReadyIndexGenerationRequiresExactHashAndTimestamp(t *testing.T) {
	t.Parallel()
	readyAt := time.Now().UTC()
	valid := SearchIndexGeneration{
		PublicationKind:         "sdk",
		PublicationID:           "publication",
		AssetKind:               "sdk",
		BuilderVersion:          "builder-v1",
		RetrievalProfileVersion: "retrieval-v1",
		State:                   "ready",
		ContentHash:             validDeveloperAssetHash,
		ReadyAt:                 &readyAt,
	}
	if !valid.Valid() {
		t.Fatal("ready exact index generation was rejected")
	}
	missingHash := valid
	missingHash.ContentHash = ""
	if missingHash.Valid() {
		t.Fatal("ready index without a content hash was accepted")
	}
	dimensions := 1536
	mismatchedEmbedding := valid
	mismatchedEmbedding.EmbeddingDimensions = &dimensions
	if mismatchedEmbedding.Valid() {
		t.Fatal("embedding dimensions without a model were accepted")
	}
}
