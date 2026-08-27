import { useTranslation } from "react-i18next";
import { RefreshCw, TriangleAlert, Users } from "lucide-react";
import { useState } from "react";

import type { APICustomerAccount } from "../../lib/api";
import { IDENTITY_TABS, identityPath, type IdentityTab } from "../../lib/console-routes";
import { Badge, Button } from "../core/control";
import { PageHeader as PageHeading, PageTabs, PanelHeader } from "../core/layout";
import { ConsoleLink } from "./console-link";

export function IdentityNavigation({ active, onNavigate }: { active: IdentityTab; onNavigate: (path: string) => void }) {
  const { t } = useTranslation();
  return <PageTabs label={t("navigation.identity")}>{IDENTITY_TABS.map((tab) => <ConsoleLink key={tab.id} path={identityPath(tab.id)} onNavigate={onNavigate} className={`page-tab ${active === tab.id ? "active" : ""}`} ariaCurrent={active === tab.id ? "page" : undefined}>{t(tab.label)}</ConsoleLink>)}</PageTabs>;
}

export function CustomerAccountsView({ accounts, status, hasMore, onUpdate, onLoadMore, onNavigate }: {
  accounts: APICustomerAccount[];
  status: "loading" | "ready" | "unavailable";
  hasMore: boolean;
  onUpdate: (account: APICustomerAccount, state: APICustomerAccount["state"]) => Promise<boolean>;
  onLoadMore: () => Promise<boolean>;
  onNavigate: (path: string) => void;
}) {
  const { t } = useTranslation();
  const [busyAccount, setBusyAccount] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);

  async function updateAccount(account: APICustomerAccount, state: APICustomerAccount["state"]) {
    if (state === "suspended" && !window.confirm(t("agentAccess.suspendCustomerMCPAccessWillFailClosedImmediately", { external_id: String(account.external_id) }))) return;
    setBusyAccount(account.id);
    try { await onUpdate(account, state); } finally { setBusyAccount(null); }
  }

  return <>
    <PageHeading eyebrow={t("navigation.identity")} title={t("navigation.customerAccounts")} />
    <IdentityNavigation active="customer-accounts" onNavigate={onNavigate} />
    <section className="panel customer-access-panel">
      <PanelHeader title={t("agentAccess.customerAccess")} description={t("agentAccess.suspendACompromisedCustomerAccountWithoutChangingTheShared")} action={status === "ready" ? <Badge color="zinc">{t("agentAccess.accounts", { count: accounts.length })}</Badge> : undefined} />
      {status === "loading" && <div className="customer-access-state"><RefreshCw /><span><strong>{t("agentAccess.loadingCustomerAccounts")}</strong></span></div>}
      {status === "unavailable" && <div className="customer-access-state unavailable"><TriangleAlert /><span><strong>{t("agentAccess.customerAccountsUnavailable")}</strong></span></div>}
      {status === "ready" && accounts.length === 0 && <div className="customer-access-empty"><Users /><span><strong>{t("agentAccess.noCustomerAccountsYet")}</strong><small>{t("agentAccess.accountsAppearAfterTheFirstSuccessfulCustomerSignIn")}</small></span></div>}
      {status === "ready" && accounts.length > 0 && <div className="customer-access-list">{accounts.map((account) => <article className="customer-access-row" key={account.id}><span className="customer-access-identity"><strong>{account.external_id}</strong><small>{t("agentAccess.lastSignIn")} {t("format.dateTime", { value: new Date(account.last_authenticated_at) })}</small></span><Badge color={account.state === "active" ? "green" : "red"}>{account.state === "active" ? t("agentAccess.active") : t("agentAccess.suspended")}</Badge><Button outline disabled={busyAccount !== null} onClick={() => void updateAccount(account, account.state === "active" ? "suspended" : "active")}>{busyAccount === account.id ? t("common.saving") : account.state === "active" ? t("agentAccess.suspend") : t("agentAccess.reactivate")}</Button></article>)}{hasMore && <Button outline disabled={loadingMore} onClick={async () => { setLoadingMore(true); try { await onLoadMore(); } finally { setLoadingMore(false); } }}>{loadingMore ? t("common.loading") : t("agentAccess.loadMore")}</Button>}</div>}
    </section>
  </>;
}
