package store

import (
	"time"

	"github.com/dokosoko/dokosoko-service/internal/model"
)

type memoryDocumentationHead struct {
	PublicationID string
	Revision      int64
}

type memoryDeveloperAssets struct {
	rawBlobs map[string]model.DeveloperAssetRawBlob

	ingestionRuns   map[string]model.DeveloperAssetIngestionRun
	ingestionStages map[string]map[string]model.DeveloperAssetIngestionStage

	documentationOutputs      map[string]DocumentationIngestionOutput
	sourcePublicationReviews  map[string]SourcePublicationDocumentationReview
	documentationCollections  map[string]model.DocumentationCollection
	documentationRevisions    map[string]DocumentationCollectionRevisionRecord
	documentationRevisionIDs  map[string][]string
	documentationPublications map[string]model.DeploymentDocumentationPublication
	documentationHeads        map[string]memoryDocumentationHead

	contracts            map[string]model.APIContract
	contractSources      map[string]model.APIContractSource
	contractCandidates   map[string]APIContractCandidateRecord
	contractCandidateIDs map[string][]string
	contractRevisions    map[string]model.APIContractRevision
	contractRevisionIDs  map[string][]string

	sdkPackages       map[string]model.SDKPackage
	sdkReleases       map[string]model.SDKRelease
	sdkReleaseIDs     map[string][]string
	sdkReleaseEvents  map[string][]model.SDKReleaseLifecycleEvent
	sdkCandidates     map[string]SDKContentCandidateRecord
	sdkCandidateIDs   map[string][]string
	sdkPublications   map[string]SDKContentPublicationRecord
	sdkPublicationIDs map[string][]string
	sdkAssertions     map[string]model.SDKCompatibilityAssertion
	sdkAssertionIDs   map[string][]string

	documentationBindings map[string]model.APIDocumentationBinding
	contractBindings      map[string]model.APIContractBinding
	sdkBindings           map[string]model.APISDKBinding
	apiPublications       map[string]model.APIDeveloperAssetPublication
	apiPublicationIDs     map[string][]string

	indexGenerations      map[string]SearchIndexGenerationRecord
	queryTraces           map[string]RetrievalQueryTraceRecord
	evaluationSets        map[string]model.RetrievalEvaluationSet
	evaluationRevisions   map[string]RetrievalEvaluationSetRevisionRecord
	evaluationRevisionIDs map[string][]string
	evaluationRuns        map[string]RetrievalEvaluationRunRecord
	evaluationRunIDs      map[string][]string
	aiAdvisoryRuns        map[string]model.DeveloperAssetAIAdvisoryRun
	aiAdvisoryInputIDs    map[string]string
}

// Developer assets are deployment-owned but the legacy Product projection is
// still used by MCP discovery and optimistic product updates. Keep both
// catalog revisions synchronized in the in-memory store, matching the single
// transactional catalog revision used by PostgreSQL.
func (m *Memory) bumpDeveloperAssetCatalogRevisionLocked() {
	now := time.Now().UTC()
	m.deployment.CatalogRevision++
	m.deployment.UpdatedAt = now
	if product, ok := m.products[m.deployment.ID]; ok {
		product.CatalogRevision = m.deployment.CatalogRevision
		product.UpdatedAt = now
		m.products[product.ID] = product
	}
}

func newMemoryDeveloperAssets() *memoryDeveloperAssets {
	return &memoryDeveloperAssets{
		rawBlobs:                  make(map[string]model.DeveloperAssetRawBlob),
		ingestionRuns:             make(map[string]model.DeveloperAssetIngestionRun),
		ingestionStages:           make(map[string]map[string]model.DeveloperAssetIngestionStage),
		documentationOutputs:      make(map[string]DocumentationIngestionOutput),
		sourcePublicationReviews:  make(map[string]SourcePublicationDocumentationReview),
		documentationCollections:  make(map[string]model.DocumentationCollection),
		documentationRevisions:    make(map[string]DocumentationCollectionRevisionRecord),
		documentationRevisionIDs:  make(map[string][]string),
		documentationPublications: make(map[string]model.DeploymentDocumentationPublication),
		documentationHeads:        make(map[string]memoryDocumentationHead),
		contracts:                 make(map[string]model.APIContract),
		contractSources:           make(map[string]model.APIContractSource),
		contractCandidates:        make(map[string]APIContractCandidateRecord),
		contractCandidateIDs:      make(map[string][]string),
		contractRevisions:         make(map[string]model.APIContractRevision),
		contractRevisionIDs:       make(map[string][]string),
		sdkPackages:               make(map[string]model.SDKPackage),
		sdkReleases:               make(map[string]model.SDKRelease),
		sdkReleaseIDs:             make(map[string][]string),
		sdkReleaseEvents:          make(map[string][]model.SDKReleaseLifecycleEvent),
		sdkCandidates:             make(map[string]SDKContentCandidateRecord),
		sdkCandidateIDs:           make(map[string][]string),
		sdkPublications:           make(map[string]SDKContentPublicationRecord),
		sdkPublicationIDs:         make(map[string][]string),
		sdkAssertions:             make(map[string]model.SDKCompatibilityAssertion),
		sdkAssertionIDs:           make(map[string][]string),
		documentationBindings:     make(map[string]model.APIDocumentationBinding),
		contractBindings:          make(map[string]model.APIContractBinding),
		sdkBindings:               make(map[string]model.APISDKBinding),
		apiPublications:           make(map[string]model.APIDeveloperAssetPublication),
		apiPublicationIDs:         make(map[string][]string),
		indexGenerations:          make(map[string]SearchIndexGenerationRecord),
		queryTraces:               make(map[string]RetrievalQueryTraceRecord),
		evaluationSets:            make(map[string]model.RetrievalEvaluationSet),
		evaluationRevisions:       make(map[string]RetrievalEvaluationSetRevisionRecord),
		evaluationRevisionIDs:     make(map[string][]string),
		evaluationRuns:            make(map[string]RetrievalEvaluationRunRecord),
		evaluationRunIDs:          make(map[string][]string),
		aiAdvisoryRuns:            make(map[string]model.DeveloperAssetAIAdvisoryRun),
		aiAdvisoryInputIDs:        make(map[string]string),
	}
}
