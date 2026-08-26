package store

import (
	"context"
	"strings"
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

// DocumentationIngestionOutput is the immutable, inspectable result of one
// documentation ingestion run. Publication review records inclusion decisions
// separately so parser output is never rewritten during approval.
type DocumentationIngestionOutput struct {
	Documents []model.DocumentationDocument
	Sections  []model.DocumentationSection
	Map       *model.DocumentationMap
}

type DocumentationCandidateQuery struct {
	DeploymentID        string
	IngestionRunID      string
	SourceID            string
	SourcePublicationID string
	QueryText           string
	Limit               int
	Offset              int
}

type DocumentationCandidateRecord struct {
	Document                    model.DocumentationDocument                `json:"document"`
	Sections                    []model.DocumentationSection               `json:"sections"`
	Run                         model.DeveloperAssetIngestionRun           `json:"run"`
	DocumentationMap            *model.DocumentationMap                    `json:"documentation_map,omitempty"`
	SourcePublicationSelections []model.SourcePublicationDocumentSelection `json:"source_publication_selections"`
}

type DocumentationCandidatePage struct {
	Items   []DocumentationCandidateRecord `json:"items"`
	Total   int                            `json:"total"`
	HasMore bool                           `json:"has_more"`
}

type DocumentationCollectionRevisionRecord struct {
	Revision model.DocumentationCollectionRevision
	Members  []model.DocumentationCollectionMember
	Map      *model.DocumentationMap
}

func snapshotDocumentationCollectionIdentity(collection model.DocumentationCollection, revision *model.DocumentationCollectionRevision) error {
	if revision == nil || strings.TrimSpace(collection.Name) == "" || strings.TrimSpace(collection.Slug) == "" {
		return ErrConflict
	}
	if revision.DocumentationCollectionName == "" {
		revision.DocumentationCollectionName = collection.Name
	}
	if revision.DocumentationCollectionSlug == "" {
		revision.DocumentationCollectionSlug = collection.Slug
	}
	if revision.DocumentationCollectionDescription == "" {
		revision.DocumentationCollectionDescription = collection.Description
	}
	if revision.DocumentationCollectionName != collection.Name ||
		revision.DocumentationCollectionSlug != collection.Slug ||
		revision.DocumentationCollectionDescription != collection.Description {
		return ErrConflict
	}
	return nil
}

func snapshotAPIContractIdentity(contract model.APIContract, revision *model.APIContractRevision) error {
	if revision == nil || strings.TrimSpace(contract.Name) == "" || strings.TrimSpace(contract.Slug) == "" || strings.TrimSpace(contract.Kind) == "" {
		return ErrConflict
	}
	if revision.APIContractName == "" {
		revision.APIContractName = contract.Name
	}
	if revision.APIContractSlug == "" {
		revision.APIContractSlug = contract.Slug
	}
	if revision.APIContractDescription == "" {
		revision.APIContractDescription = contract.Description
	}
	if revision.APIContractKind == "" {
		revision.APIContractKind = contract.Kind
	}
	if revision.APIContractName != contract.Name || revision.APIContractSlug != contract.Slug ||
		revision.APIContractDescription != contract.Description || revision.APIContractKind != contract.Kind {
		return ErrConflict
	}
	return nil
}

func snapshotAPIDocumentationAssetIdentity(revision model.DocumentationCollectionRevision, asset *model.APIPublicationDocumentationAsset) error {
	if asset == nil {
		return ErrConflict
	}
	if asset.DocumentationCollectionID == "" {
		asset.DocumentationCollectionID = revision.DocumentationCollectionID
	}
	if asset.DocumentationCollectionName == "" {
		asset.DocumentationCollectionName = revision.DocumentationCollectionName
	}
	if asset.DocumentationCollectionSlug == "" {
		asset.DocumentationCollectionSlug = revision.DocumentationCollectionSlug
	}
	if asset.DocumentationCollectionDescription == "" {
		asset.DocumentationCollectionDescription = revision.DocumentationCollectionDescription
	}
	if asset.DocumentationCollectionID != revision.DocumentationCollectionID ||
		asset.DocumentationCollectionName != revision.DocumentationCollectionName ||
		asset.DocumentationCollectionSlug != revision.DocumentationCollectionSlug ||
		asset.DocumentationCollectionDescription != revision.DocumentationCollectionDescription {
		return ErrConflict
	}
	return nil
}

func snapshotAPIContractAssetIdentity(revision model.APIContractRevision, asset *model.APIPublicationContractAsset) error {
	if asset == nil {
		return ErrConflict
	}
	if asset.APIContractID == "" {
		asset.APIContractID = revision.APIContractID
	}
	if asset.APIContractName == "" {
		asset.APIContractName = revision.APIContractName
	}
	if asset.APIContractSlug == "" {
		asset.APIContractSlug = revision.APIContractSlug
	}
	if asset.APIContractDescription == "" {
		asset.APIContractDescription = revision.APIContractDescription
	}
	if asset.APIContractKind == "" {
		asset.APIContractKind = revision.APIContractKind
	}
	if asset.APIContractID != revision.APIContractID || asset.APIContractName != revision.APIContractName ||
		asset.APIContractSlug != revision.APIContractSlug || asset.APIContractDescription != revision.APIContractDescription ||
		asset.APIContractKind != revision.APIContractKind {
		return ErrConflict
	}
	return nil
}

type SourcePublicationDocumentationReview struct {
	Selections []model.SourcePublicationDocumentSelection
	MapLink    *model.SourcePublicationDocumentationMap
}

type APIContractCandidateRecord struct {
	Candidate  model.APIContractCandidate
	Operations []model.APIContractOperation
	Schemas    []model.APIContractSchema
	Examples   []model.APIContractExample
	Map        *model.APIContractMap
}

type SDKContentCandidateRecord struct {
	Candidate  model.SDKContentCandidate     `json:"candidate"`
	Files      []model.SDKPublicationFile    `json:"files"`
	Sections   []model.SDKSection            `json:"sections"`
	Symbols    []model.SDKSymbol             `json:"symbols"`
	Samples    []model.SDKCodeSample         `json:"samples"`
	Map        *model.SDKMap                 `json:"map,omitempty"`
	SampleRefs []model.SDKSampleAPIReference `json:"sample_refs"`
}

// SDKContentIngestionFinalization is the single commit boundary for the
// immutable candidate graph, its completed stage evidence, and the owning
// run's transition into review_ready.
type SDKContentIngestionFinalization struct {
	Candidate        SDKContentCandidateRecord
	Stages           []model.DeveloperAssetIngestionStage
	Run              model.DeveloperAssetIngestionRun
	ExpectedRunState model.DeveloperAssetIngestionState
}

type SDKContentPublicationRecord struct {
	Publication      model.SDKContentPublication                  `json:"publication"`
	FileSelections   []model.SDKContentPublicationFileSelection   `json:"file_selections"`
	SampleSelections []model.SDKContentPublicationSampleSelection `json:"sample_selections"`
	Map              *model.SDKContentPublicationMap              `json:"map,omitempty"`
	PublishedMap     *model.SDKMap                                `json:"published_map,omitempty"`
}

type SearchIndexGenerationRecord struct {
	Generation model.SearchIndexGeneration
	Units      []model.KnowledgeUnit
	APIScopes  []model.KnowledgeUnitAPIScope
}

type DeveloperAssetKnowledgeQuery struct {
	DeploymentID                         string
	DeploymentDocumentationPublicationID string
	APIDeveloperAssetPublicationID       string
	APIID                                string
	BuilderVersion                       string
	RetrievalProfileVersion              string
	AssetKinds                           []string
	Languages                            []string
	Ecosystems                           []string
	SDKReleaseIDs                        []string
	ExactVersions                        []string
	QueryText                            string
	QueryEmbedding                       []float32
	Limit                                int
}

type DeveloperAssetKnowledgeResult struct {
	Unit          model.KnowledgeUnit
	LexicalScore  float64
	SemanticScore float64
	FusedScore    float64
}

type RetrievalQueryTraceRecord struct {
	Trace   model.RetrievalQueryTrace
	Results []model.RetrievalQueryTraceResult
}

// DeveloperAssetUsageRecord is one deployment-wide snapshot of API bindings
// and immutable publications. Catalog views use it to resolve "Used by APIs"
// without issuing one request per API and asset kind.
type DeveloperAssetUsageRecord struct {
	Documentation []model.APIDocumentationBinding
	Contracts     []model.APIContractBinding
	SDKs          []model.APISDKBinding
	Publications  []model.APIDeveloperAssetPublication
}

type DeveloperAssetAIAdvisoryQuery struct {
	DeploymentID string
	PromptKey    string
	ScopeID      string
	Limit        int
}

// DeveloperAssetIngestionStore persists the state shared by all developer-asset
// ingestion pipelines.
type DeveloperAssetIngestionStore interface {
	DeveloperAssetIngestionRuns(context.Context, string, model.DeveloperAssetKind, string) ([]model.DeveloperAssetIngestionRun, error)
	DeveloperAssetIngestionRun(context.Context, string, string) (model.DeveloperAssetIngestionRun, error)
	CreateDeveloperAssetIngestionRun(context.Context, model.DeveloperAssetIngestionRun) (model.DeveloperAssetIngestionRun, error)
	TransitionDeveloperAssetIngestionRun(context.Context, model.DeveloperAssetIngestionRun, model.DeveloperAssetIngestionState) (model.DeveloperAssetIngestionRun, error)
	DeveloperAssetIngestionStages(context.Context, string) ([]model.DeveloperAssetIngestionStage, error)
	SaveDeveloperAssetIngestionStage(context.Context, model.DeveloperAssetIngestionStage, string) (model.DeveloperAssetIngestionStage, error)
}

// DeveloperAssetDocumentationStore owns normalized documentation candidates,
// revisions, and deployment publications.
type DeveloperAssetDocumentationStore interface {
	DocumentationIngestionOutput(context.Context, string, string) (DocumentationIngestionOutput, error)
	SaveDocumentationIngestionOutput(context.Context, string, DocumentationIngestionOutput) error
	DocumentationCandidateDocuments(context.Context, DocumentationCandidateQuery) (DocumentationCandidatePage, error)
	DocumentationCandidateDocument(context.Context, string, string) (DocumentationCandidateRecord, error)
	DocumentationCandidateSection(context.Context, string, string) (model.DocumentationSection, DocumentationCandidateRecord, error)
	SourcePublicationDocumentationReview(context.Context, string, string) (SourcePublicationDocumentationReview, error)
	SaveSourcePublicationDocumentationReview(context.Context, string, SourcePublicationDocumentationReview) error

	DocumentationCollections(context.Context, string) ([]model.DocumentationCollection, error)
	DocumentationCollection(context.Context, string, string) (model.DocumentationCollection, error)
	CreateDocumentationCollection(context.Context, model.DocumentationCollection, DocumentationCollectionRevisionRecord) (model.DocumentationCollection, error)
	ReviseDocumentationCollection(context.Context, model.DocumentationCollection, int64, DocumentationCollectionRevisionRecord) (model.DocumentationCollection, error)
	DocumentationCollectionRevisions(context.Context, string, string) ([]model.DocumentationCollectionRevision, error)
	DocumentationCollectionRevision(context.Context, string, string) (DocumentationCollectionRevisionRecord, error)

	DeploymentDocumentationPublications(context.Context, string) ([]model.DeploymentDocumentationPublication, error)
	DeploymentDocumentationPublication(context.Context, string, string) (model.DeploymentDocumentationPublication, error)
	ActiveDeploymentDocumentationPublication(context.Context, string) (model.DeploymentDocumentationPublication, int64, error)
	PublishDeploymentDocumentation(context.Context, model.DeploymentDocumentationPublication, int64) (model.DeploymentDocumentationPublication, error)
}

// DeveloperAssetContractStore owns API contracts, their source associations,
// ingestion candidates, and immutable revisions.
type DeveloperAssetContractStore interface {
	APIContracts(context.Context, string) ([]model.APIContract, error)
	APIContract(context.Context, string, string) (model.APIContract, error)
	SaveAPIContract(context.Context, model.APIContract, int64) (model.APIContract, error)
	APIContractSources(context.Context, string, string) ([]model.APIContractSource, error)
	APIContractSource(context.Context, string, string) (model.APIContractSource, error)
	ActiveAPIContractSourceBySource(context.Context, string, string) (model.APIContractSource, error)
	SaveAPIContractSource(context.Context, model.APIContractSource, int64) (model.APIContractSource, error)
	DetachAPIContractSource(context.Context, string, string, int64) (model.APIContractSource, error)
	APIContractCandidates(context.Context, string, string) ([]model.APIContractCandidate, error)
	APIContractCandidate(context.Context, string, string) (APIContractCandidateRecord, error)
	CreateAPIContractCandidate(context.Context, APIContractCandidateRecord) (model.APIContractCandidate, error)
	APIContractRevisions(context.Context, string, string) ([]model.APIContractRevision, error)
	APIContractRevision(context.Context, string, string) (model.APIContractRevision, error)
	PublishAPIContractCandidate(context.Context, model.APIContract, int64, model.APIContractRevision, *model.APIContractRevisionSourcePublication) (model.APIContract, model.APIContractRevision, error)
}

// DeveloperAssetSDKStore owns the shared SDK catalog and its ingestion,
// publication, lifecycle, and compatibility records.
type DeveloperAssetSDKStore interface {
	SDKPackages(context.Context, string) ([]model.SDKPackage, error)
	SDKPackage(context.Context, string, string) (model.SDKPackage, error)
	SaveSDKPackage(context.Context, model.SDKPackage, int64) (model.SDKPackage, error)
	SDKReleases(context.Context, string, string) ([]model.SDKRelease, error)
	SDKRelease(context.Context, string, string) (model.SDKRelease, error)
	CreateSDKRelease(context.Context, model.SDKRelease) (model.SDKRelease, error)
	SDKReleaseLifecycleEvents(context.Context, string, string) ([]model.SDKReleaseLifecycleEvent, error)
	AppendSDKReleaseLifecycleEvent(context.Context, string, SDKReleaseLifecycleMutation) (model.SDKReleaseLifecycleEvent, error)
	SDKContentCandidates(context.Context, string, string) ([]model.SDKContentCandidate, error)
	SDKContentCandidate(context.Context, string, string) (SDKContentCandidateRecord, error)
	CreateSDKContentCandidate(context.Context, SDKContentCandidateRecord) (model.SDKContentCandidate, error)
	FinalizeSDKContentIngestion(context.Context, SDKContentIngestionFinalization) (model.SDKContentCandidate, model.DeveloperAssetIngestionRun, error)
	SDKContentPublications(context.Context, string, string) ([]model.SDKContentPublication, error)
	SDKContentPublication(context.Context, string, string) (SDKContentPublicationRecord, error)
	PublishSDKContentCandidate(context.Context, SDKContentPublicationRecord) (model.SDKContentPublication, error)
	SDKCompatibilityAssertions(context.Context, string, string, string) ([]model.SDKCompatibilityAssertion, error)
	SDKCompatibilityAssertion(context.Context, string, string) (model.SDKCompatibilityAssertion, error)
	CreateSDKCompatibilityAssertion(context.Context, model.SDKCompatibilityAssertion) (model.SDKCompatibilityAssertion, error)
}

// DeveloperAssetBindingStore owns API-to-asset relationships and the immutable
// snapshots published from those relationships.
type DeveloperAssetBindingStore interface {
	DeveloperAssetUsage(context.Context, string) (DeveloperAssetUsageRecord, error)
	APIDocumentationBindings(context.Context, string, string) ([]model.APIDocumentationBinding, error)
	APIDocumentationBinding(context.Context, string, string, string) (model.APIDocumentationBinding, error)
	SaveAPIDocumentationBinding(context.Context, model.APIDocumentationBinding, int64) (model.APIDocumentationBinding, error)
	DetachAPIDocumentationBinding(context.Context, string, string, string, int64) (model.APIDocumentationBinding, error)
	APIContractBindings(context.Context, string, string) ([]model.APIContractBinding, error)
	APIContractBinding(context.Context, string, string, string) (model.APIContractBinding, error)
	SaveAPIContractBinding(context.Context, model.APIContractBinding, int64) (model.APIContractBinding, error)
	DetachAPIContractBinding(context.Context, string, string, string, int64) (model.APIContractBinding, error)
	APISDKBindings(context.Context, string, string) ([]model.APISDKBinding, error)
	APISDKBinding(context.Context, string, string, string) (model.APISDKBinding, error)
	SaveAPISDKBinding(context.Context, model.APISDKBinding, int64) (model.APISDKBinding, error)
	DetachAPISDKBinding(context.Context, string, string, string, int64) (model.APISDKBinding, error)

	APIDeveloperAssetPublications(context.Context, string, string) ([]model.APIDeveloperAssetPublication, error)
	APIDeveloperAssetPublication(context.Context, string, string) (model.APIDeveloperAssetPublication, error)
	CreateAPIDeveloperAssetPublication(context.Context, model.APIDeveloperAssetPublication) (model.APIDeveloperAssetPublication, error)
}

// DeveloperAssetRetrievalStore owns search generations, retrieval, and retained
// query traces. Evaluation-run persistence is intentionally not part of this
// contract until an executable evaluation workflow exists.
type DeveloperAssetRetrievalStore interface {
	SearchIndexGenerations(context.Context, string, string, string) ([]model.SearchIndexGeneration, error)
	SearchIndexGeneration(context.Context, string, string) (SearchIndexGenerationRecord, error)
	CreateSearchIndexGeneration(context.Context, model.SearchIndexGeneration) (model.SearchIndexGeneration, error)
	CompleteSearchIndexGeneration(context.Context, SearchIndexGenerationRecord, string) (model.SearchIndexGeneration, error)
	FailSearchIndexGeneration(context.Context, model.SearchIndexGeneration, string) (model.SearchIndexGeneration, error)
	RetrieveDeveloperAssetKnowledge(context.Context, DeveloperAssetKnowledgeQuery) ([]DeveloperAssetKnowledgeResult, error)

	RetrievalQueryTraces(context.Context, string, time.Time, int) ([]model.RetrievalQueryTrace, error)
	RetrievalQueryTrace(context.Context, string, string) (RetrievalQueryTraceRecord, error)
	AppendRetrievalQueryTrace(context.Context, RetrievalQueryTraceRecord) error
	DeleteExpiredRetrievalQueryTraces(context.Context, time.Time, int) (int64, error)
}

// DeveloperAssetAIStore persists advisory runs independently from deterministic
// ingestion and publication state.
type DeveloperAssetAIStore interface {
	DeveloperAssetAIAdvisoryRuns(context.Context, DeveloperAssetAIAdvisoryQuery) ([]model.DeveloperAssetAIAdvisoryRun, error)
	DeveloperAssetAIAdvisoryRun(context.Context, string, string) (model.DeveloperAssetAIAdvisoryRun, error)
	DeveloperAssetAIAdvisoryRunByInputHash(context.Context, string, string, string) (model.DeveloperAssetAIAdvisoryRun, error)
	CreateDeveloperAssetAIAdvisoryRun(context.Context, model.DeveloperAssetAIAdvisoryRun) (model.DeveloperAssetAIAdvisoryRun, error)
}

// DeveloperAssetStore is the complete control-plane persistence contract for
// reusable developer assets. Aggregate creation methods are transactional:
// callers never observe a candidate, revision, publication, index generation,
// or trace with only some of its immutable children present.
type DeveloperAssetStore interface {
	DeveloperAssetIngestionStore
	DeveloperAssetDocumentationStore
	DeveloperAssetContractStore
	DeveloperAssetSDKStore
	DeveloperAssetBindingStore
	DeveloperAssetRetrievalStore
	DeveloperAssetAIStore
}
