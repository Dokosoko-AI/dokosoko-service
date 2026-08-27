"use client";


import { useTranslation } from "react-i18next";
import { CheckCircle2, ChevronRight, TriangleAlert, XCircle } from "lucide-react";
import type { ReactNode } from "react";
import { Badge } from "../core/control";
import { PanelHeader } from "../core/layout";

export type IntegrationSetupStep = {
  label: string;
  detail: string;
  ready: boolean;
  path: string;
};

export type IntegrationSetupValidation = {
  code: string;
  level: "warning" | "error";
  message: string;
  path: string;
};

function WorkspaceLink({ path, onNavigate, className, children }: {
  path: string;
  onNavigate: (path: string) => void;
  className: string;
  children: ReactNode;
}) {
  return <a href={path} className={className} onClick={(event) => {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onNavigate(path);
  }}>{children}</a>;
}

export function IntegrationQuickStart({
  lifecycle,
  status,
  statusDetail,
  steps,
  validations,
  advanced,
  onNavigate,
}: {
  lifecycle: string;
  status: "checking" | "ready" | "published" | "setup";
  statusDetail: string;
  steps: IntegrationSetupStep[];
  validations: IntegrationSetupValidation[];
  advanced: ReactNode;
  onNavigate: (path: string) => void;
}) {
  const { t } = useTranslation();
  const readyCount = steps.filter((step) => step.ready).length;
  const nextStep = steps.findIndex((step) => !step.ready);
  const statusLabel = status === "checking" ? t("integrationQuickStart.checkingStatus") : status === "ready" ? t("integrationQuickStart.readyToPublish") : status === "published" ? t("integrationQuickStart.published") : t("integrationQuickStart.needsSetup");
  const lifecycleColor = lifecycle === "active" ? "green" : lifecycle === "deprecated" ? "amber" : "zinc";

  return <div className="integration-tab-content integration-quick-start">
    <div className="api-status-bar">
      <span><span className={`status-dot${status === "checking" ? " checking" : ""}`} /><strong>{statusLabel}</strong><small>{statusDetail}</small></span>
      <Badge color={lifecycleColor}>{lifecycle === "active" ? t("integrationQuickStart.active") : lifecycle === "deprecated" ? t("integrationQuickStart.deprecated") : lifecycle === "archived" ? t("integrationQuickStart.archived") : lifecycle}</Badge>
    </div>
    <section className="panel onboarding-checklist">
      <PanelHeader
        title={t("integrationQuickStart.getYourAPIReady")}
        description={t("integrationQuickStart.followTheShortestPathToATestablePublishableAPI")}
        action={<Badge color={readyCount === steps.length ? "green" : "violet"}>{readyCount}/{steps.length}</Badge>}
      />
      {steps.map((step, index) => {
        const isNext = index === nextStep;
        return <WorkspaceLink key={step.label} path={step.path} onNavigate={onNavigate} className={`integration-health-check${isNext ? " next" : ""}`}>
          <span className={`health-icon ${step.ready ? "ready" : ""}`}>{step.ready ? <CheckCircle2 /> : <span className="step-number">{index + 1}</span>}</span>
          <span><strong>{step.label}</strong><small>{step.detail}</small></span>
          <Badge color={step.ready ? "green" : isNext ? "violet" : "zinc"}>{step.ready ? t("integrationQuickStart.ready") : isNext ? t("integrationQuickStart.next") : t("integrationQuickStart.setup")}</Badge>
          <ChevronRight />
        </WorkspaceLink>;
      })}
    </section>
    <details className="panel advanced-details quick-start-advanced">
      <summary>{t("integrationQuickStart.optionalSetupAndAPIDetails")}</summary>
      <div className="advanced-details-body">
        {validations.length > 0 && <section className="panel quick-start-validation-list">
          <PanelHeader title={t("integrationQuickStart.publicationDetails")} description={t("integrationQuickStart.additionalFindingsFromTheCurrentCandidateSnapshot")} />
          {validations.map((validation) => <WorkspaceLink key={validation.code} path={validation.path} onNavigate={onNavigate} className={`publish-validation ${validation.level}`}>
            <span>{validation.level === "error" ? <XCircle /> : <TriangleAlert />}</span>
            <span><strong>{validation.level === "error" ? t("integrationQuickStart.resolveBeforePublishing") : t("integrationQuickStart.reviewBeforePublishing")}</strong><small>{validation.message}</small></span>
            <ChevronRight />
          </WorkspaceLink>)}
        </section>}
        {advanced}
      </div>
    </details>
  </div>;
}
