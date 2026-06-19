import type { ReactNode } from "react";
import { cn } from "../lib/cn";
import styles from "./SectionHeader.module.css";

export interface SectionHeaderProps {
  icon?: ReactNode;
  title: string;
  /** "查看全部" style action. */
  actionLabel?: string;
  onAction?: () => void;
  className?: string;
}

export function SectionHeader({
  icon,
  title,
  actionLabel,
  onAction,
  className,
}: SectionHeaderProps) {
  return (
    <div className={cn(styles.header, className)}>
      <div className={styles.titleWrap}>
        {icon ? <span className={styles.icon}>{icon}</span> : null}
        <h2 className={styles.title}>{title}</h2>
      </div>
      {actionLabel ? (
        <button type="button" className={styles.action} onClick={onAction}>
          {actionLabel}
          <span aria-hidden className={styles.chev}>
            ›
          </span>
        </button>
      ) : null}
    </div>
  );
}
