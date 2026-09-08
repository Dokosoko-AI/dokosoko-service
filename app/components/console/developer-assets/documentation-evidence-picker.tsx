"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api, type APISource, type APISourcePublication } from "../../../lib/api";
import { developerAssetsApi, type DocumentationCandidateRecord, type DocumentationCollectionMemberInput, type DocumentationLibraryItem } from "../../../lib/developer-assets-api";
import { Button } from "../../core/control";
import { EvidenceContent } from "../../core/evidence-content";
import { developerAssetError } from "./developer-asset-ui";

export type DocumentationEvidenceSelection = { member: DocumentationCollectionMemberInput; label: string; visibility: "private" | "public" };

// Only names and exact reviewed versions are selectable. IDs remain transport
// values; documents come from a bounded query of this publication's inclusions.
export function DocumentationEvidencePicker({ deploymentID, wholeSourceOnly = false, publicOnly = false, onChange }: {
  deploymentID?: string; wholeSourceOnly?: boolean; publicOnly?: boolean;
  onChange: (selection: DocumentationEvidenceSelection | null) => void;
}) {
  const { t } = useTranslation();
  const [productID, setProductID] = useState(deploymentID ?? "");
  const [sources, setSources] = useState<APISource[]>([]);
  const [sourceID, setSourceID] = useState("");
  const [sourcesLoaded, setSourcesLoaded] = useState(false);
  const [publicationsLoadedFor, setPublicationsLoadedFor] = useState("");
  const [publications, setPublications] = useState<APISourcePublication[]>([]);
  const [publicationID, setPublicationID] = useState("");
  const [kind, setKind] = useState<DocumentationCollectionMemberInput["kind"]>("source_publication");
  const [documents, setDocuments] = useState<DocumentationLibraryItem[]>([]);
  const [documentID, setDocumentID] = useState("");
  const [record, setRecord] = useState<DocumentationCandidateRecord | null>(null);
  const [sectionID, setSectionID] = useState("");
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(false);
  const [problem, setProblem] = useState("");
  const [attempt, setAttempt] = useState(0);
  const source = sources.find((item) => item.id === sourceID);
  const publication = publications.find((item) => item.id === publicationID && (!publicOnly || item.visibility === "public"));
  const label = publication && source ? `${source.name} · r${publication.revision}` : "";

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const id = deploymentID || (await api.deployment()).id;
      const values = await api.sources(id);
      if (!cancelled) { setProductID(id); setSources(values); setSourcesLoaded(true); setProblem(""); }
    })().catch((error) => { if (!cancelled) setProblem(developerAssetError(error, t("evidencePicker.unavailable"))); });
    return () => { cancelled = true; };
  }, [attempt, deploymentID, t]);

  useEffect(() => {
    if (!productID || !sourceID) return;
    let cancelled = false;
    api.sourcePublications(productID, sourceID).then((values) => { if (!cancelled) { setPublications(values); setPublicationsLoadedFor(sourceID); setProblem(""); } }).catch((error) => { if (!cancelled) setProblem(developerAssetError(error, t("evidencePicker.unavailable"))); });
    return () => { cancelled = true; };
  }, [attempt, productID, sourceID, t]);

  useEffect(() => {
    if (!publicationID || kind === "source_publication") return;
    let cancelled = false;
    developerAssetsApi.documentationLibrary({ source_publication_id: publicationID, view: "history", query: search, offset, limit: 50 }).then((page) => {
      if (!cancelled) { setDocuments((current) => offset ? [...current, ...page.items.filter((item) => !current.some((other) => other.id === item.id))] : page.items); setHasMore(page.has_more); setProblem(""); }
    }).catch((error) => { if (!cancelled) setProblem(developerAssetError(error, t("evidencePicker.unavailable"))); });
    return () => { cancelled = true; };
  }, [attempt, kind, offset, publicationID, search, t]);

  useEffect(() => {
    if (!documentID) return;
    let cancelled = false;
    developerAssetsApi.documentationDocument(documentID).then((value) => { if (!cancelled) { setRecord(value); setProblem(""); } }).catch((error) => { if (!cancelled) setProblem(developerAssetError(error, t("evidencePicker.unavailable"))); });
    return () => { cancelled = true; };
  }, [attempt, documentID, t]);

  function selectMember(nextKind: DocumentationCollectionMemberInput["kind"], id: string, title: string, visibility: "private" | "public") {
    onChange({ member: { kind: nextKind, id, include_descendants: true, selector: {} }, label: title, visibility });
  }
  function clearDocument() { setDocumentID(""); setSectionID(""); setRecord(null); setDocuments([]); setOffset(0); setHasMore(false); }
  const eligibleSources = sources.filter((item) => !item.quarantined && (!publicOnly || item.visibility === "public"));
  const currentRecord = record?.document.id === documentID ? record : null;

  return <div className="auth-form compact-form">
    {problem && <div role="alert"><p className="auth-problem">{problem}</p><Button outline onClick={() => setAttempt((value) => value + 1)}>{t("common.retry")}</Button></div>}
    <label className="auth-field"><span>{t("evidencePicker.source")}</span><select value={sourceID} onChange={(event) => { setSourceID(event.target.value); setPublications([]); setPublicationID(""); clearDocument(); setProblem(""); onChange(null); }}><option value="">{t("evidencePicker.chooseSource")}</option>{eligibleSources.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}</select>{sourcesLoaded && eligibleSources.length === 0 && <small>{t("evidencePicker.noSources")}</small>}</label>
    {sourceID && <label className="auth-field"><span>{t("evidencePicker.publication")}</span><select disabled={publicationsLoadedFor !== sourceID} value={publicationID} onChange={(event) => { const value = publications.find((item) => item.id === event.target.value); setPublicationID(event.target.value); clearDocument(); onChange(null); if (value && source && kind === "source_publication") selectMember(kind, value.id, `${source.name} · r${value.revision}`, value.visibility); }}><option value="">{t(publicationsLoadedFor === sourceID ? "evidencePicker.chooseRevision" : "common.loading")}</option>{publications.filter((item) => !publicOnly || item.visibility === "public").map((item) => <option key={item.id} value={item.id}>{t("evidencePicker.revision", { revision: item.revision, count: item.document_count })} · {t("format.dateTime", { value: new Date(item.published_at) })}</option>)}</select></label>}
    {publication && <>
      {!wholeSourceOnly && <label className="auth-field"><span>{t("evidencePicker.include")}</span><select value={kind} onChange={(event) => { const value = event.target.value as DocumentationCollectionMemberInput["kind"]; setKind(value); clearDocument(); onChange(null); if (value === "source_publication") selectMember(value, publication.id, label, publication.visibility); }}><option value="source_publication">{t("evidencePicker.wholeSource")}</option><option value="document">{t("evidencePicker.oneDocument")}</option><option value="section">{t("evidencePicker.oneSection")}</option></select></label>}
      {kind !== "source_publication" && <>
        <div className="two-fields"><label className="auth-field"><span>{t("evidencePicker.search")}</span><input value={query} maxLength={500} onChange={(event) => setQuery(event.target.value)} /></label><Button outline onClick={() => { clearDocument(); setSearch(query); setAttempt((value) => value + 1); onChange(null); }}>{t("evidencePicker.search")}</Button></div>
        <label className="auth-field"><span>{t("evidencePicker.document")}</span><select value={documentID} onChange={(event) => { const item = documents.find((value) => value.id === event.target.value); setDocumentID(event.target.value); setSectionID(""); setRecord(null); onChange(null); if (item && kind === "document") selectMember(kind, item.id, `${item.title} · ${label}`, item.visibility); }}><option value="">{t("evidencePicker.chooseDocument")}</option>{documents.map((item) => <option key={item.id} value={item.id}>{item.title} · {item.source_path}</option>)}</select>{documents.length === 0 && <small>{t("evidencePicker.noDocuments")}</small>}</label>
        {hasMore && <Button outline onClick={() => setOffset(documents.length)}>{t("evidencePicker.more")}</Button>}
        {kind === "section" && currentRecord && <label className="auth-field"><span>{t("evidencePicker.section")}</span><select value={sectionID} onChange={(event) => { setSectionID(event.target.value); const section = currentRecord.sections.find((item) => item.id === event.target.value); onChange(null); if (section) selectMember(kind, section.id, `${section.heading || t("evidencePicker.untitledSection")} · ${currentRecord.document.title}`, currentRecord.document.visibility); }}><option value="">{t("evidencePicker.chooseSection")}</option>{currentRecord.sections.map((section) => <option key={section.id} value={section.id}>{section.heading || t("evidencePicker.untitledSection")}</option>)}</select></label>}
        {currentRecord && <details><summary>{t("sdkCatalog.readExactContent")}</summary><EvidenceContent text={currentRecord.document.normalized_markdown} label={currentRecord.document.title} /></details>}
      </>}
      <p>{t("evidencePicker.frozen")}</p>
    </>}
  </div>;
}
