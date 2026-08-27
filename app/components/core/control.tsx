"use client";

import * as Headless from "@headlessui/react";
import clsx from "clsx";
import type { ButtonHTMLAttributes, HTMLAttributes, ReactNode } from "react";
import { Badge as BaseBadge } from "./badge";
import { Button as BaseButton } from "./button";

type ButtonProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "color"> & {
  color?: "dark" | "red" | "indigo";
  outline?: boolean;
  children: ReactNode;
};

export function Button({ color = "dark", outline = false, className, children, ...props }: ButtonProps) {
  const classes = clsx(
    "core-button",
    outline ? "core-button-outline" : `core-button-${color}`,
    className,
  );

  if (outline) {
    return <BaseButton {...props} outline className={classes}>{children}</BaseButton>;
  }

  return <BaseButton {...props} color={color} className={classes}>{children}</BaseButton>;
}

export function Badge({ color = "zinc", className, children, ...props }: {
  color?: "zinc" | "green" | "blue" | "violet" | "amber" | "red";
  className?: string;
  children: ReactNode;
} & Omit<HTMLAttributes<HTMLSpanElement>, "color">) {
  return (
    <BaseBadge {...props} color={color} className={clsx("core-badge", `core-badge-${color}`, className)}>
      {children}
    </BaseBadge>
  );
}

export function Switch({ checked, onChange, disabled = false, label }: {
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
      className="core-switch"
    >
      <span aria-hidden="true" />
    </Headless.Switch>
  );
}

export function Dialog({ open, onClose, title, description, children, actions }: {
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
