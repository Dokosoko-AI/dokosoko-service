import { buildDocumentationMap, documentationMapEnrichmentInput } from "./documentation-map";
import { compareText } from "./canonical";
import { normalizeDocument } from "./normalize";
import { assessCorpusQuality } from "./quality";
import { segmentDocument } from "./segment";
import type {
  DocumentationMap,
  DocumentationMapEnrichmentInput,
  NormalizationInput,
  NormalizedDocument,
  NormalizedSection,
  QualityDiagnostic,
  SegmentationOptions,
} from "./types";

export type DocumentationCorpus = {
  documents: readonly NormalizedDocument[];
  sections: readonly NormalizedSection[];
  diagnostics: readonly QualityDiagnostic[];
  map: DocumentationMap;
  enrichmentInput: DocumentationMapEnrichmentInput;
};

export function buildDocumentationCorpus(
  inputs: readonly NormalizationInput[],
  segmentation: SegmentationOptions = {},
): DocumentationCorpus {
  const documents = inputs.map(normalizeDocument)
    .sort((left, right) => compareText(left.source.canonicalUrl, right.source.canonicalUrl) || compareText(left.id, right.id));
  const segmented = documents.map((document) => segmentDocument(document, segmentation));
  const sections = segmented.flatMap((result) => result.sections);
  const diagnostics = assessCorpusQuality(documents, sections, segmented.flatMap((result) => result.diagnostics));
  const map = buildDocumentationMap(documents, sections, diagnostics);
  return {
    documents,
    sections,
    diagnostics,
    map,
    enrichmentInput: documentationMapEnrichmentInput(map, documents, sections),
  };
}
