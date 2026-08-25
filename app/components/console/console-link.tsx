import type { MouseEvent, ReactNode } from "react";

import { entityPath, type EntityKind } from "../../lib/console-routes";

export function ConsoleLink({ path, onNavigate, className, children, ariaCurrent, ariaLabel }: {
  path: string;
  onNavigate: (path: string) => void;
  className?: string;
  children: ReactNode;
  ariaCurrent?: "page";
  ariaLabel?: string;
}) {
  function navigate(event: MouseEvent<HTMLAnchorElement>) {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onNavigate(path);
  }

  return <a href={path} className={className} aria-current={ariaCurrent} aria-label={ariaLabel} onClick={navigate}>{children}</a>;
}

export function EntityLink({ entity, uid, onNavigate, className, children }: {
  entity: EntityKind;
  uid: string;
  onNavigate: (path: string) => void;
  className?: string;
  children: ReactNode;
}) {
  return <ConsoleLink path={entityPath(entity, uid)} onNavigate={onNavigate} className={className}>{children}</ConsoleLink>;
}
