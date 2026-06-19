import type { ReactNode } from "react";
import { GlassPanel } from "./GlassPanel";
import styles from "./EmptyState.module.css";

export interface EmptyStateProps {
  icon?: ReactNode;
  title: string;
  description?: string;
  /** Optional action node (e.g. a link to import/manage). */
  action?: ReactNode;
}

/** A tasteful empty state for empty data sections. */
export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <GlassPanel variant="subtle" className={styles.panel}>
      <div className={styles.content}>
        {icon ? <div className={styles.icon}>{icon}</div> : null}
        <div>
          <p className={styles.title}>{title}</p>
          {description ? <p className={styles.desc}>{description}</p> : null}
        </div>
      </div>
      {action ? <div className={styles.action}>{action}</div> : null}
    </GlassPanel>
  );
}
