"use client";


import { useTranslation } from "react-i18next";
import { ChevronDown, GitBranch } from "lucide-react";
import {
  INTEGRATION_PRIMARY_TABS,
  type IntegrationTab,
  integrationPath,
} from "../../lib/console-routes";
import {
  Dropdown,
  DropdownButton,
  DropdownDescription,
  DropdownItem,
  DropdownLabel,
  DropdownMenu,
} from "../core/dropdown";
import { PageTabs } from "../core/layout";

function WorkspaceLink({
  path,
  active,
  onNavigate,
  children,
}: {
  path: string;
  active: boolean;
  onNavigate: (path: string) => void;
  children: React.ReactNode;
}) {
  return <a
    href={path}
    className={`page-tab ${active ? "active" : ""}`}
    aria-current={active ? "page" : undefined}
    onClick={(event) => {
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      event.preventDefault();
      onNavigate(path);
    }}
  >{children}</a>;
}

export function IntegrationNavigation({
  integrationID,
  integrationName,
  activeTab,
  onNavigate,
}: {
  integrationID: string;
  integrationName: string;
  activeTab: IntegrationTab;
  onNavigate: (path: string) => void;
}) {
  const { t } = useTranslation();
  const historyPath = integrationPath(integrationID, "history");
  const historyActive = activeTab === "history";

  return <PageTabs label={t("integrationNavigation.sections", { integrationName: String(integrationName) })}>
    {INTEGRATION_PRIMARY_TABS.map((tab) => <WorkspaceLink
      key={tab.id}
      path={integrationPath(integrationID, tab.id)}
      active={activeTab === tab.id}
      onNavigate={onNavigate}
    >{t(tab.label)}</WorkspaceLink>)}
    <Dropdown>
      <DropdownButton
        as="button"
        className={`page-tab integration-more-tab ${historyActive ? "active" : ""}`}
        aria-label={t("integrationNavigation.moreAPISections")}
        aria-current={historyActive ? "page" : undefined}
      > {t("integrationNavigation.more")} <ChevronDown data-slot="icon" />
      </DropdownButton>
      <DropdownMenu anchor="bottom end" className="integration-more-menu">
        <DropdownItem onClick={() => onNavigate(historyPath)}>
          <GitBranch data-slot="icon" />
          <DropdownLabel>{t("integrationNavigation.history")}</DropdownLabel>
          <DropdownDescription>{t("integrationNavigation.publishedRevisionsAndImmutableSnapshots")}</DropdownDescription>
        </DropdownItem>
      </DropdownMenu>
    </Dropdown>
  </PageTabs>;
}
