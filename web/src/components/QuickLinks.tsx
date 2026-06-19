import type { QuickLink } from "../lib/types";
import { QUICK_LINKS } from "../features/home/mockData";
import styles from "./QuickLinks.module.css";

export interface QuickLinksProps {
  items?: QuickLink[];
  /**
   * Optional recorder fired (fire-and-forget) when a tile is opened, so the
   * pinned → favorite → click-count ordering can become more accurate over
   * time. It must never block the normal external navigation.
   */
  onRecordOpen?: (id: string) => void;
  /** Optional caption reinforcing how the tiles are ordered / managed. */
  hint?: string;
}

/** A row of frequently-used shortcut tiles. Horizontally scrollable on mobile.
 *
 *  Tiles use a single calm, neutral treatment (no per-domain colour) so the
 *  row reads as one cohesive surface ready for future customisation. Ordering
 *  is owned by the data layer (`deriveQuickLinks`: pinned → favorite →
 *  click-count desc); clicks are reported back when possible. */
export function QuickLinks({ items = QUICK_LINKS, onRecordOpen, hint }: QuickLinksProps) {
  return (
    <div>
      <div className={styles.row} role="list">
        {items.map((item) => (
          <a
            key={item.id}
            className={styles.tile}
            href={item.url}
            target="_blank"
            rel="noreferrer noopener"
            role="listitem"
            title={item.label}
            onClick={() => onRecordOpen?.(item.id)}
          >
            <span className={styles.badge}>
              <span className={styles.ring}>{item.icon}</span>
            </span>
            <span className={styles.label}>{item.label}</span>
          </a>
        ))}
      </div>
      {hint ? <p className={styles.hint}>{hint}</p> : null}
    </div>
  );
}
