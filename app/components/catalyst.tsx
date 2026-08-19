"use client";

import * as Headless from "@headlessui/react";
import clsx from "clsx";
import type { ButtonHTMLAttributes, ReactNode } from "react";

export function Button({
  color = "dark",
  outline = false,
  className,
  children,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  color?: "dark" | "red" | "indigo";
  outline?: boolean;
  children: ReactNode;
}) {
  return (
    <Headless.Button
      {...props}
      className={clsx(
        "catalyst-button",
        outline ? "catalyst-button-outline" : `catalyst-button-${color}`,
        className,
      )}
    >
      {children}
    </Headless.Button>
  );
}

export function Badge({
  color = "zinc",
  className,
  children,
}: {
  color?: "zinc" | "green" | "blue" | "violet" | "amber" | "red";
  className?: string;
  children: ReactNode;
}) {
  return <span className={clsx("catalyst-badge", `catalyst-badge-${color}`, className)}>{children}</span>;
}

export function Switch({
  checked,
  onChange,
  disabled = false,
  label,
}: {
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  label: string;
}) {
  return (
    <Headless.Switch
      checked={checked}
      onChange={onChange}
      disabled={disabled}
      aria-label={label}
      className="catalyst-switch"
    >
      <span aria-hidden="true" />
    </Headless.Switch>
  );
}

export function Dialog({
  open,
  onClose,
  title,
  description,
  children,
  actions,
}: {
  open: boolean;
  onClose: (open: boolean) => void;
  title: string;
  description: string;
  children?: ReactNode;
  actions: ReactNode;
}) {
  return (
    <Headless.Dialog open={open} onClose={onClose} className="dialog-root">
      <Headless.DialogBackdrop transition className="dialog-backdrop" />
      <div className="dialog-scroll">
        <Headless.DialogPanel transition className="dialog-panel">
          <Headless.DialogTitle className="dialog-title">{title}</Headless.DialogTitle>
          <Headless.Description className="dialog-description">{description}</Headless.Description>
          {children && <div className="dialog-body">{children}</div>}
          <div className="dialog-actions">{actions}</div>
        </Headless.DialogPanel>
      </div>
    </Headless.Dialog>
  );
}

