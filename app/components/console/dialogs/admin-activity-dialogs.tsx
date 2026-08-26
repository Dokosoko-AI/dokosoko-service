"use client";

import { ShieldCheck } from "lucide-react";

import { Badge, Button, Dialog } from "../../core/control";
import type { useAdminActivityWorkspace } from "../use-admin-activity-workspace";

export function AdminActivityDialogs({ workspace }: {
  workspace: ReturnType<typeof useAdminActivityWorkspace>;
}) {
  const {
    reportDetail, setReportDetail,
    reportDetailBusy,
    rootOpen, setRootOpen,
    rootBusy,
    rootEmail, setRootEmail,
    rootDisplayName, setRootDisplayName,
    rootPassword, setRootPassword,
    rootCode, setRootCode,
    rootEnrollment,
    rootRecoveryCodes, setRootRecoveryCodes,
    beginRootUser,
    completeRootUser,
  } = workspace;

  return <>
    <Dialog
      open={Boolean(reportDetail)}
      onClose={(open) => { if (!open) setReportDetail(null); }}
      title={reportDetail?.kind === "bug" ? "Bug report" : "Feedback submission"}
      description="Plaintext, schema-bounded content approved by the reporting user."
      actions={<Button color="indigo" onClick={() => setReportDetail(null)}>Close</Button>}
    >
      {reportDetailBusy ? <div className="empty-row">Loading submission…</div> : reportDetail && <div className="report-detail"><div className="report-detail-meta"><span><small>Status</small><Badge color="blue">{reportDetail.state}</Badge></span><span><small>API</small><code>{reportDetail.trusted_integration?.display_name || "Deployment"}</code></span><span><small>Created</small><strong>{new Date(reportDetail.created_at).toLocaleString()}</strong></span></div><pre>{JSON.stringify(reportDetail.content ?? { summary: reportDetail.summary }, null, 2)}</pre></div>}
    </Dialog>

    <Dialog
      open={rootOpen}
      onClose={setRootOpen}
      title="Add root administrator"
      description="Every root administrator has a unique strong password, TOTP enrollment, recovery codes, and independently revocable sessions."
      actions={rootRecoveryCodes.length ? <Button color="indigo" onClick={() => { setRootOpen(false); setRootRecoveryCodes([]); setRootEmail(""); setRootDisplayName(""); setRootPassword(""); }}>I saved the recovery codes</Button> : rootEnrollment ? <><Button outline onClick={() => setRootOpen(false)}>Cancel</Button><Button color="indigo" disabled={rootBusy || rootCode.length !== 6} onClick={completeRootUser}>{rootBusy ? "Verifying…" : "Create root"}</Button></> : <><Button outline onClick={() => setRootOpen(false)}>Cancel</Button><Button color="indigo" disabled={rootBusy || !rootEmail.trim() || !rootDisplayName.trim() || rootPassword.length < 14} onClick={beginRootUser}>{rootBusy ? "Preparing…" : "Continue to MFA"}</Button></>}
    >
      {rootRecoveryCodes.length ? <div className="auth-form compact-form"><div className="private-default-note"><ShieldCheck />These one-time recovery codes are shown once. Store them in a secure password manager.</div><div className="recovery-grid">{rootRecoveryCodes.map((code) => <code key={code}>{code}</code>)}</div></div> : rootEnrollment ? <div className="auth-form compact-form"><label className="auth-field"><span>Authenticator secret</span><input readOnly value={rootEnrollment.totp_secret} onFocus={(event) => event.currentTarget.select()} /><small>Add this secret to the new administrator&apos;s authenticator. Enrollment expires in 15 minutes.</small></label><label className="auth-field"><span>6-digit verification code</span><input inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={rootCode} onChange={(event) => setRootCode(event.target.value.replace(/\D/g, ""))} /></label></div> : <div className="auth-form compact-form"><label className="auth-field"><span>Email</span><input type="email" value={rootEmail} onChange={(event) => setRootEmail(event.target.value)} /></label><label className="auth-field"><span>Display name</span><input value={rootDisplayName} onChange={(event) => setRootDisplayName(event.target.value)} /></label><label className="auth-field"><span>Initial password</span><input type="password" autoComplete="new-password" value={rootPassword} onChange={(event) => setRootPassword(event.target.value)} /><small>At least 14 characters with upper/lower-case and a number.</small></label></div>}
    </Dialog>
  </>;
}
