"use client";

import { useState } from "react";

import {
  APIError,
  api,
  type APIAuditEvent,
  type APISupportSubmission,
  type APIUser,
  type SetupEnrollment,
} from "../../lib/api";

export function useAdminActivityWorkspace({ currentUser, apiConnected, showToast }: {
  currentUser?: APIUser | null;
  apiConnected: boolean;
  showToast: (message: string) => void;
}) {
  const [reportSubmissions, setReportSubmissions] = useState<APISupportSubmission[]>([]);
  const [reportDetail, setReportDetail] = useState<APISupportSubmission | null>(null);
  const [reportDetailBusy, setReportDetailBusy] = useState(false);
  const [rootUsers, setRootUsers] = useState<APIUser[]>(currentUser ? [currentUser] : []);
  const [rootOpen, setRootOpen] = useState(false);
  const [rootBusy, setRootBusy] = useState(false);
  const [rootEmail, setRootEmail] = useState("");
  const [rootDisplayName, setRootDisplayName] = useState("");
  const [rootPassword, setRootPassword] = useState("");
  const [rootCode, setRootCode] = useState("");
  const [rootEnrollment, setRootEnrollment] = useState<SetupEnrollment | null>(null);
  const [rootRecoveryCodes, setRootRecoveryCodes] = useState<string[]>([]);
  const [auditEvents, setAuditEvents] = useState<APIAuditEvent[]>([]);

  async function openSupportSubmission(submission: APISupportSubmission) {
    setReportDetail(submission);
    if (!apiConnected) return;
    setReportDetailBusy(true);
    try {
      setReportDetail(await api.supportSubmission(submission.id));
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not load this submission.");
      setReportDetail(null);
    } finally {
      setReportDetailBusy(false);
    }
  }

  async function beginRootUser() {
    setRootBusy(true);
    try {
      const value = await api.beginRootUser({ email: rootEmail, display_name: rootDisplayName, password: rootPassword });
      setRootEnrollment(value);
      setRootCode("");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not start root enrollment.");
    } finally {
      setRootBusy(false);
    }
  }

  async function completeRootUser() {
    if (!rootEnrollment) return;
    setRootBusy(true);
    try {
      const value = await api.completeRootUser(rootEnrollment.enrollment_id, rootCode);
      setRootUsers((items) => [...items, value.user]);
      setRootRecoveryCodes(value.recovery_codes);
      setRootEnrollment(null);
      setRootCode("");
      showToast("MFA-protected root administrator created.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "MFA verification failed.");
    } finally {
      setRootBusy(false);
    }
  }

  async function revokeRootUser(user: APIUser) {
    if (!window.confirm(`Revoke root access for ${user.email}? Their active sessions will end immediately.`)) return;
    try {
      await api.revokeRootUser(user.id);
      setRootUsers((items) => items.map((item) => item.id === user.id ? { ...item, revoked_at: new Date().toISOString() } : item));
      showToast(`${user.email} was revoked.`);
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not revoke root administrator.");
    }
  }

  return {
    reportSubmissions, setReportSubmissions,
    reportDetail, setReportDetail,
    reportDetailBusy,
    rootUsers, setRootUsers,
    rootOpen, setRootOpen,
    rootBusy,
    rootEmail, setRootEmail,
    rootDisplayName, setRootDisplayName,
    rootPassword, setRootPassword,
    rootCode, setRootCode,
    rootEnrollment,
    rootRecoveryCodes, setRootRecoveryCodes,
    auditEvents, setAuditEvents,
    openSupportSubmission,
    beginRootUser,
    completeRootUser,
    revokeRootUser,
  };
}
