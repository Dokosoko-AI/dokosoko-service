"use client";

import { useState } from "react";

import {
  APIError,
  api,
  type APIAnalytics,
  type APIAuditEvent,
  type APIEnvironment,
  type APIIntegrationRun,
  type APIProduct,
  type APISupportSubmission,
  type APIUser,
  type SetupEnrollment,
} from "../../lib/api";
import type { ConsoleFixtures } from "../../dev/console-fixtures";

export function useAdminActivityWorkspace({ product, fixtures, currentUser, apiConnected, showToast }: {
  product: APIProduct;
  fixtures?: ConsoleFixtures;
  currentUser?: APIUser | null;
  apiConnected: boolean;
  showToast: (message: string) => void;
}) {
  const [analytics, setAnalytics] = useState<APIAnalytics | null>(null);
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
  const [environments, setEnvironments] = useState<APIEnvironment[]>(fixtures ? [fixtures.environment] : []);
  const [integrationRuns, setIntegrationRuns] = useState<APIIntegrationRun[]>([]);
  const [auditEvents, setAuditEvents] = useState<APIAuditEvent[]>([]);
  const [runOpen, setRunOpen] = useState(false);
  const [runBusy, setRunBusy] = useState(false);
  const [runEnvironmentID, setRunEnvironmentID] = useState(fixtures?.environment.id ?? "");
  const [runOutcome, setRunOutcome] = useState("");

  async function createSupportDeliveryAttempt(submission: APISupportSubmission) {
    try {
      const value = apiConnected ? await api.createSupportDeliveryAttempt(submission.id) : { ...submission, state: "pending" as const, last_error: undefined };
      setReportSubmissions((items) => items.map((item) => item.id === value.id ? value : item));
      showToast("Submission queued for another delivery attempt.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not retry this submission.");
    }
  }

  async function openSupportSubmission(submission: APISupportSubmission) {
    setReportDetail(submission);
    if (!apiConnected) return;
    setReportDetailBusy(true);
    try {
      setReportDetail(await api.supportSubmission(submission.id));
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not decrypt this submission.");
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

  async function startIntegrationRun() {
    setRunBusy(true);
    try {
      const value = apiConnected
        ? await api.startIntegrationRun(product.id, runEnvironmentID, runOutcome)
        : { id: `run_${Date.now()}`, organisation_id: product.organisation_id, product_id: product.id, environment_id: runEnvironmentID, requested_outcome: runOutcome, state: "running" as const, started_at: new Date().toISOString() };
      setIntegrationRuns((items) => [value, ...items]);
      setRunOpen(false);
      setRunOutcome("");
      showToast("API run started.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not start API run.");
    } finally {
      setRunBusy(false);
    }
  }

  async function completeIntegrationRun(run: APIIntegrationRun, succeeded: boolean) {
    const failureCode = succeeded ? "" : window.prompt("Failure code (for example validation_failed)", "validation_failed")?.trim() ?? "";
    if (!succeeded && !failureCode) return;
    try {
      const value = apiConnected
        ? await api.completeIntegrationRun(product.id, run.id, succeeded, succeeded, failureCode)
        : { ...run, state: succeeded ? "succeeded" as const : "failed" as const, reported_success: succeeded, validated_success: succeeded, failure_code: failureCode || undefined, finished_at: new Date().toISOString() };
      setIntegrationRuns((items) => items.map((item) => item.id === value.id ? value : item));
      if (apiConnected) setAnalytics(await api.analytics(product.id));
      showToast(succeeded ? "Validated success recorded." : "Validated failure recorded for diagnosis.");
    } catch (error) {
      showToast(error instanceof APIError ? error.message : "Could not complete API run.");
    }
  }

  return {
    analytics, setAnalytics,
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
    environments, setEnvironments,
    integrationRuns, setIntegrationRuns,
    auditEvents, setAuditEvents,
    runOpen, setRunOpen,
    runBusy,
    runEnvironmentID, setRunEnvironmentID,
    runOutcome, setRunOutcome,
    createSupportDeliveryAttempt,
    openSupportSubmission,
    beginRootUser,
    completeRootUser,
    revokeRootUser,
    startIntegrationRun,
    completeIntegrationRun,
  };
}
