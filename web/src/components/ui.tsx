import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from "react";
import { cn } from "../lib/cn";
import styles from "./ui.module.css";

// =====================================================================
// Button
// =====================================================================

type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
type ButtonSize = "md" | "sm";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  /** Leading icon. */
  icon?: ReactNode;
  loading?: boolean;
}

export function Button({
  variant = "secondary",
  size = "md",
  icon,
  loading = false,
  className,
  children,
  disabled,
  ...rest
}: ButtonProps) {
  return (
    <button
      type="button"
      className={cn(
        styles.btn,
        styles[variant],
        size === "sm" && styles.sm,
        loading && styles.loading,
        className,
      )}
      disabled={disabled || loading}
      {...rest}
    >
      {icon ? <span className={styles.btnIcon}>{icon}</span> : null}
      {children}
    </button>
  );
}

// =====================================================================
// IconButton
// =====================================================================

export interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  /** Tooltip/accessibility label. */
  label: string;
  variant?: "glass" | "danger";
  active?: boolean;
}

export function IconButton({
  label,
  variant = "glass",
  active = false,
  className,
  children,
  ...rest
}: IconButtonProps) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={cn(styles.iconBtn, variant === "danger" && styles.iconDanger, active && styles.iconActive, className)}
      {...rest}
    >
      {children}
    </button>
  );
}

// =====================================================================
// Form fields
// =====================================================================

export interface FormFieldProps {
  label: string;
  hint?: string;
  required?: boolean;
  error?: string | null;
  children: ReactNode;
  className?: string;
}

export function FormField({ label, hint, required, error, children, className }: FormFieldProps) {
  return (
    <label className={cn(styles.field, className)}>
      <span className={styles.fieldLabel}>
        {label}
        {required ? <span className={styles.req} aria-hidden>*</span> : null}
      </span>
      {children}
      {error ? <span className={styles.fieldError} role="alert">{error}</span> : null}
      {hint && !error ? <span className={styles.fieldHint}>{hint}</span> : null}
    </label>
  );
}

export function TextInput({ className, invalid, ...rest }: InputHTMLAttributes<HTMLInputElement> & { invalid?: boolean }) {
  return <input className={cn(styles.input, invalid && styles.inputInvalid, className)} {...rest} />;
}

export function TextArea({ className, ...rest }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea className={cn(styles.input, styles.textarea, className)} {...rest} />;
}

export function Select({ className, children, ...rest }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <div className={styles.selectWrap}>
      <select className={cn(styles.input, styles.select, className)} {...rest}>
        {children}
      </select>
    </div>
  );
}

// =====================================================================
// Checkbox pill (for toggles like pinned/favorite)
// =====================================================================

export interface CheckChipProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  label: string;
  icon?: ReactNode;
  disabled?: boolean;
}

export function CheckChip({ checked, onChange, label, icon, disabled }: CheckChipProps) {
  return (
    <button
      type="button"
      className={cn(styles.chip, checked && styles.chipOn)}
      disabled={disabled}
      aria-pressed={checked}
      onClick={() => onChange(!checked)}
    >
      {icon ? <span className={styles.chipIcon}>{icon}</span> : null}
      {label}
    </button>
  );
}
