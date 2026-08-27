import { useTranslation } from "react-i18next";
import { useState, type FormEvent } from "react";

import type { APIProduct } from "../../lib/api";
import { Button } from "../core/control";
import { Input, Textarea } from "../core";
import { PageHeader as PageHeading, PanelHeader } from "../core/layout";
import { SettingsTabs } from "./catalog-settings-views";

const tenantSlugPattern = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

function validSubmissionURL(value: string) {
  if (!value) return true;
  if (value.length > 2048) return false;
  try {
    const parsed = new URL(value);
    const hostname = parsed.hostname.toLowerCase();
    const local = hostname === "localhost" || hostname.endsWith(".localhost") || hostname === "::1" || /^127(?:\.\d{1,3}){3}$/.test(hostname);
    if (!hostname || parsed.username || parsed.password || parsed.search || parsed.hash) return false;
    return local ? parsed.protocol === "http:" : parsed.protocol === "https:" && (parsed.port === "" || parsed.port === "443");
  } catch {
    return false;
  }
}

export function TenantSettingsView({ product, onSave, onNavigate }: {
  product: APIProduct;
  onSave: (input: { name: string; slug: string; description: string; feedback_submission_url: string; error_submission_url: string }) => Promise<boolean>;
  onNavigate: (path: string) => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(product.name);
  const [slug, setSlug] = useState(product.slug);
  const [description, setDescription] = useState(product.description);
  const [feedbackSubmissionURL, setFeedbackSubmissionURL] = useState(product.feedback_submission_url ?? "");
  const [errorSubmissionURL, setErrorSubmissionURL] = useState(product.error_submission_url ?? "");
  const [saving, setSaving] = useState(false);

  const trimmedName = name.trim();
  const trimmedSlug = slug.trim();
  const trimmedDescription = description.trim();
  const trimmedFeedbackSubmissionURL = feedbackSubmissionURL.trim();
  const trimmedErrorSubmissionURL = errorSubmissionURL.trim();
  const submissionURLsValid = validSubmissionURL(trimmedFeedbackSubmissionURL) && validSubmissionURL(trimmedErrorSubmissionURL);
  const valid = trimmedName.length > 0 && trimmedName.length <= 120 && trimmedSlug.length <= 63 && tenantSlugPattern.test(trimmedSlug) && trimmedDescription.length <= 2000 && submissionURLsValid;
  const changed = trimmedName !== product.name || trimmedSlug !== product.slug || trimmedDescription !== product.description || trimmedFeedbackSubmissionURL !== (product.feedback_submission_url ?? "") || trimmedErrorSubmissionURL !== (product.error_submission_url ?? "");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!valid || !changed || saving) return;
    setSaving(true);
    try {
      await onSave({ name: trimmedName, slug: trimmedSlug, description: trimmedDescription, feedback_submission_url: trimmedFeedbackSubmissionURL, error_submission_url: trimmedErrorSubmissionURL });
    } finally {
      setSaving(false);
    }
  }

  return <>
    <PageHeading eyebrow={t("navigation.settings")} title={t("routes.tenantSettings")} />
    <SettingsTabs active="tenant" onNavigate={onNavigate} />
    <section className="panel tenant-settings-panel">
      <PanelHeader title={t("tenantSettings.profileTitle")} description={t("tenantSettings.profileDescription")} />
      <form className="auth-form compact-form tenant-settings-form" onSubmit={(event) => void submit(event)}>
        <div className="two-fields">
          <label className="auth-field" htmlFor="tenant-name"><span>{t("tenantSettings.name")}</span><Input id="tenant-name" maxLength={120} value={name} invalid={Boolean(name) && !trimmedName} onChange={(event) => setName(event.target.value)} /><small>{t("tenantSettings.nameHint")}</small></label>
          <label className="auth-field" htmlFor="tenant-slug"><span>{t("tenantSettings.slug")}</span><Input id="tenant-slug" maxLength={63} pattern="[a-z0-9]+(?:-[a-z0-9]+)*" autoCapitalize="none" autoCorrect="off" spellCheck={false} value={slug} invalid={Boolean(slug) && !tenantSlugPattern.test(trimmedSlug)} onChange={(event) => setSlug(event.target.value.toLowerCase())} /><small>{t("tenantSettings.slugHint")}</small></label>
        </div>
        <label className="auth-field" htmlFor="tenant-description"><span>{t("tenantSettings.description")}</span><Textarea id="tenant-description" rows={5} maxLength={2000} value={description} onChange={(event) => setDescription(event.target.value)} /><small>{t("tenantSettings.descriptionHint")} {description.length}/2000</small></label>
        <div className="tenant-settings-subsection">
          <header><h3>{t("tenantSettings.submissionEndpointsTitle")}</h3><p>{t("tenantSettings.submissionEndpointsDescription")}</p></header>
          <div className="two-fields">
            <label className="auth-field" htmlFor="feedback-submission-url"><span>{t("tenantSettings.feedbackSubmissionURL")}</span><Input id="feedback-submission-url" type="url" maxLength={2048} autoCapitalize="none" autoCorrect="off" spellCheck={false} placeholder="https://support.example.com/feedback" value={feedbackSubmissionURL} invalid={Boolean(trimmedFeedbackSubmissionURL) && !validSubmissionURL(trimmedFeedbackSubmissionURL)} onChange={(event) => setFeedbackSubmissionURL(event.target.value)} /><small>{t("tenantSettings.feedbackSubmissionURLHint")}</small></label>
            <label className="auth-field" htmlFor="error-submission-url"><span>{t("tenantSettings.errorSubmissionURL")}</span><Input id="error-submission-url" type="url" maxLength={2048} autoCapitalize="none" autoCorrect="off" spellCheck={false} placeholder="https://support.example.com/errors" value={errorSubmissionURL} invalid={Boolean(trimmedErrorSubmissionURL) && !validSubmissionURL(trimmedErrorSubmissionURL)} onChange={(event) => setErrorSubmissionURL(event.target.value)} /><small>{t("tenantSettings.errorSubmissionURLHint")}</small></label>
          </div>
        </div>
        <div className="tenant-settings-actions"><small>{!submissionURLsValid ? t("tenantSettings.submissionURLValidation") : !valid ? t("tenantSettings.validation") : t("tenantSettings.saveHint")}</small><Button type="submit" disabled={!valid || !changed || saving}>{saving ? t("common.saving") : t("tenantSettings.save")}</Button></div>
      </form>
    </section>
    <section className="panel tenant-settings-panel">
      <PanelHeader title={t("tenantSettings.identifiersTitle")} description={t("tenantSettings.identifiersDescription")} />
      <dl className="entity-detail-grid compact-detail-grid">
        <div><dt>{t("tenantSettings.tenantID")}</dt><dd><code>{product.id}</code></dd></div>
        <div><dt>{t("tenantSettings.organisationID")}</dt><dd><code>{product.organisation_id}</code></dd></div>
        <div><dt>{t("tenantSettings.catalogRevision")}</dt><dd>{product.catalog_revision}</dd></div>
        <div><dt>{t("tenantSettings.configurationRevision")}</dt><dd>{product.revision}</dd></div>
      </dl>
    </section>
  </>;
}
