"use client";

import { TriangleAlert } from "lucide-react";
import { useId } from "react";

import type { APIIntegrationAnalysis } from "../../lib/api";
import { Badge } from "../core/control";

export function IntegrationEvidenceGaps({ unknowns }: { unknowns: APIIntegrationAnalysis["unknowns"] }) {
  const headingID = useId();
  if (unknowns.length === 0) return null;

  const blockingCount = unknowns.filter((unknown) => unknown.blocking).length;
  return <section className="integration-evidence-gaps" aria-labelledby={headingID}>
    <TriangleAlert aria-hidden="true" />
    <div>
      <h3 id={headingID}>{blockingCount > 0 ? "Evidence gaps block generation" : "Open evidence questions"}</h3>
      <p>{blockingCount > 0 ? "Attach or configure the missing source of truth, then generate again to run a fresh analysis." : "These questions do not block generation, but resolving them will make the guidance more precise."}</p>
      <ul>
        {unknowns.map((unknown) => <li key={unknown.id}>
          <span><strong>{unknown.question}</strong><small>{unknown.why}</small></span>
          <Badge color={unknown.blocking ? "amber" : "zinc"}>{unknown.blocking ? "Blocking" : "Advisory"}</Badge>
        </li>)}
      </ul>
    </div>
  </section>;
}
