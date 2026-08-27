"use client";


import { useTranslation } from "react-i18next";
import { ShieldCheck } from "lucide-react";

import { Badge, Button, Dialog } from "../../core/control";
import type { useAdminActivityWorkspace } from "../use-admin-activity-workspace";

export function AdminActivityDialogs({ workspace }: {
  workspace: ReturnType<typeof useAdminActivityWorkspace>;
}) {
  const { t } = useTranslation();
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
      title={reportDetail?.kind === "bug" ? t("adminDialogs.bugReport") : t("adminDialogs.feedbackSubmission")}
      description={t("adminDialogs.plaintextSchemaBoundedContentApprovedByTheReportingUser")}
      actions={<Button color="indigo" onClick={() => setReportDetail(null)}>{t("common.close")}</Button>}
    >
      {reportDetailBusy ? <div className="empty-row">{t("adminDialogs.loadingSubmission")}</div> : reportDetail && <div className="report-detail"><div className="report-detail-meta"><span><small>{t("adminDialogs.status")}</small><Badge color="blue">{reportDetail.state}</Badge></span><span><small>API</small><code>{reportDetail.trusted_integration?.display_name || "Deployment"}</code></span><span><small>{t("adminDialogs.created")}</small><strong>{t("format.dateTime", { value: new Date(reportDetail.created_at) })}</strong></span></div><pre>{JSON.stringify(reportDetail.content ?? { summary: reportDetail.summary }, null, 2)}</pre></div>}
    </Dialog>

    <Dialog
      open={rootOpen}
      onClose={setRootOpen}
      title={t("adminDialogs.addRootAdministrator")}
      description={t("adminDialogs.everyRootAdministratorHasAUniqueStrongPasswordTOTP")}
      actions={rootRecoveryCodes.length ? <Button color="indigo" onClick={() => { setRootOpen(false); setRootRecoveryCodes([]); setRootEmail(""); setRootDisplayName(""); setRootPassword(""); }}>{t("adminDialogs.iSavedTheRecoveryCodes")}</Button> : rootEnrollment ? <><Button outline onClick={() => setRootOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={rootBusy || rootCode.length !== 6} onClick={completeRootUser}>{rootBusy ? t("common.verifying") : t("adminDialogs.createRoot")}</Button></> : <><Button outline onClick={() => setRootOpen(false)}>{t("common.cancel")}</Button><Button color="indigo" disabled={rootBusy || !rootEmail.trim() || !rootDisplayName.trim() || rootPassword.length < 14} onClick={beginRootUser}>{rootBusy ? t("adminDialogs.preparing") : t("adminDialogs.continueToMFA")}</Button></>}
    >
      {rootRecoveryCodes.length ? <div className="auth-form compact-form"><div className="private-default-note"><ShieldCheck />{t("adminDialogs.theseOneTimeRecoveryCodesAreShownOnceStore")}</div><div className="recovery-grid">{rootRecoveryCodes.map((code) => <code key={code}>{code}</code>)}</div></div> : rootEnrollment ? <div className="auth-form compact-form"><label className="auth-field"><span>{t("adminDialogs.authenticatorSecret")}</span><input readOnly value={rootEnrollment.totp_secret} onFocus={(event) => event.currentTarget.select()} /><small>{t("adminDialogs.addThisSecretToTheNewAdministratorSAuthenticator")}</small></label><label className="auth-field"><span>{t("adminDialogs.n6DigitVerificationCode")}</span><input inputMode="numeric" autoComplete="one-time-code" maxLength={6} value={rootCode} onChange={(event) => setRootCode(event.target.value.replace(/\D/g, ""))} /></label></div> : <div className="auth-form compact-form"><label className="auth-field"><span>{t("adminDialogs.email")}</span><input type="email" value={rootEmail} onChange={(event) => setRootEmail(event.target.value)} /></label><label className="auth-field"><span>{t("adminDialogs.displayName")}</span><input value={rootDisplayName} onChange={(event) => setRootDisplayName(event.target.value)} /></label><label className="auth-field"><span>{t("adminDialogs.initialPassword")}</span><input type="password" autoComplete="new-password" value={rootPassword} onChange={(event) => setRootPassword(event.target.value)} /><small>{t("adminDialogs.atLeastN14CharactersWithUpperLowerCaseAnd")}</small></label></div>}
    </Dialog>
  </>;
}
