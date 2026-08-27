"use client";

import { Plus, Trash2 } from "lucide-react";

import { Button } from "../core/control";

export type AuthorizationHeaderDraft = {
  id: string;
  name: string;
  value: string;
};

let nextHeaderID = 0;

export function authorizationHeaderDraft(name = "", value = ""): AuthorizationHeaderDraft {
  nextHeaderID += 1;
  return { id: `authorization-header-${nextHeaderID}`, name, value };
}

export function AuthorizationHeaderManager({ headers, onChange, required = false }: {
  headers: AuthorizationHeaderDraft[];
  onChange: (headers: AuthorizationHeaderDraft[]) => void;
  required?: boolean;
}) {
  return <div className="authorization-header-manager">
    <div className="authorization-header-heading">
      <span><strong>Headers</strong><small>{required ? "At least one header is required for this method." : "Optional fixed headers sent with authenticated requests."}</small></span>
      <Button outline disabled={headers.length >= 16} onClick={() => onChange([...headers, authorizationHeaderDraft()])}><Plus data-slot="icon" />Add header</Button>
    </div>
    {headers.length === 0 ? <div className="authorization-header-empty">No headers configured.</div> : <div className="authorization-header-list">
      {headers.map((header, index) => <div className="authorization-header-row" key={header.id}>
        <label className="auth-field"><span>Header</span><input aria-label={`Header ${index + 1} name`} value={header.name} maxLength={100} onChange={(event) => onChange(headers.map((candidate) => candidate.id === header.id ? { ...candidate, name: event.target.value } : candidate))} placeholder="X-API-Key" /></label>
        <label className="auth-field"><span>Value</span><input aria-label={`Header ${index + 1} value`} type="password" value={header.value} maxLength={16384} onChange={(event) => onChange(headers.map((candidate) => candidate.id === header.id ? { ...candidate, value: event.target.value } : candidate))} placeholder="************" autoComplete="new-password" /></label>
        <Button outline aria-label={`Delete header ${index + 1}`} onClick={() => onChange(headers.filter((candidate) => candidate.id !== header.id))}><Trash2 data-slot="icon" />Delete</Button>
      </div>)}
    </div>}
  </div>;
}
