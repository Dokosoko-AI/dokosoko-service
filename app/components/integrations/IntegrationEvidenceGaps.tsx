"use client";


import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { useId } from "react";

import type { APIIntegrationAnalysis } from "../../lib/api";
import { Badge } from "../core/control";

export function IntegrationEvidenceGaps({ unknowns }: { unknowns: APIIntegrationAnalysis["unknowns"] }) {
  const { t } = useTranslation();
  const headingID = useId();
  if (unknowns.length === 0) return null;

  const blockingCount = unknowns.filter((unknown) => unknown.blocking).length;
  return <section className="integration-evidence-gaps" aria-labelledby={headingID}>
    <TriangleAlert aria-hidden="true" />
    <div>
      <h3 id={headingID}>{blockingCount > 0 ? t("integrationEvidence.evidenceGapsBlockGeneration") : t("integrationEvidence.openEvidenceQuestions")}</h3>
      <p>{blockingCount > 0 ? t("integrationEvidence.attachOrConfigureTheMissingSourceOfTruthThen") : t("integrationEvidence.theseQuestionsDoNotBlockGenerationButResolvingThem")}</p>
      <ul>
        {unknowns.map((unknown) => <li key={unknown.id}>
          <span><strong>{unknown.question}</strong><small>{unknown.why}</small></span>
          <Badge color={unknown.blocking ? "amber" : "zinc"}>{unknown.blocking ? t("integrationEvidence.blocking") : t("integrationEvidence.advisory")}</Badge>
        </li>)}
      </ul>
    </div>
  </section>;
}
