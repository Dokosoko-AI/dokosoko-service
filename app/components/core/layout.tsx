import { clsx } from "clsx";
import { Children, cloneElement, isValidElement } from "react";
import type { ReactElement, ReactNode } from "react";

export function ViewStack({ children, className }: { children: ReactNode; className?: string }) {
  return <div className={clsx("view-shell", className)}>{children}</div>;
}

export function PageHeader({ eyebrow, title, description, action }: { eyebrow: string; title: string; description?: string; action?: ReactNode }) {
  return <header className="page-heading"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1>{description && <p>{description}</p>}</div>{action}</header>;
}

export function PageTabs({ label, children }: { label: string; children: ReactNode }) {
  return <nav className="page-tabs" aria-label={label}>{children}</nav>;
}

export function SectionHeader({ title, description, action }: { title: string; description?: string; action?: ReactNode }) {
  return <header className="section-heading"><div><h2>{title}</h2>{description && <p>{description}</p>}</div>{action}</header>;
}

export function PanelHeader({ title, description, action, level = 2, className }: { title: ReactNode; description?: ReactNode; action?: ReactNode; level?: 2 | 3; className?: string }) {
  const Heading = level === 3 ? "h3" : "h2";
  return <header className={clsx("panel-heading", className)}><div><Heading>{title}</Heading>{description && <p>{description}</p>}</div>{action}</header>;
}

export function SegmentedControl<T extends string>({ label, items, value, onChange }: { label: string; items: ReadonlyArray<{ id: T; label: string; count?: number }>; value: T; onChange: (value: T) => void }) {
  return <div className="segmented-control" role="group" aria-label={label}>{items.map((item) => <button type="button" key={item.id} className={value === item.id ? "active" : ""} aria-pressed={value === item.id} onClick={() => onChange(item.id)}>{item.label}{item.count !== undefined && <span>{item.count}</span>}</button>)}</div>;
}

function withRole(children: ReactNode, role: "cell" | "columnheader") {
  return Children.map(children, (child) => {
    if (!isValidElement(child)) return child;
    const element = child as ReactElement<{ role?: string }>;
    return cloneElement(element, { role: element.props.role ?? role });
  });
}

export function DataTable({ label, className, children }: { label: string; className?: string; children: ReactNode }) {
  return <div className={clsx("resource-table", className)} role="table" aria-label={label}>{children}</div>;
}

export function DataTableHeader({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={clsx("table-head", className)} role="row">{withRole(children, "columnheader")}</div>;
}

export function DataTableRow({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={clsx("table-row", className)} role="row">{withRole(children, "cell")}</div>;
}

export function DataTableEmpty({ children, columns = 1 }: { children: ReactNode; columns?: number }) {
  return <div className="empty-row" role="row"><span role="cell" aria-colspan={columns}>{children}</span></div>;
}
