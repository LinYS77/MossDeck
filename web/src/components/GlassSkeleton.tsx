import { GlassPanel } from "./GlassPanel";
import styles from "./GlassSkeleton.module.css";

/** Frosted placeholder block with a subtle shimmer, sized via props.
 *  Used while API data is loading to keep the premium feel. */
export function SkeletonBlock({
  width = "100%",
  height = "1em",
  radius = "var(--radius-sm)",
}: {
  width?: string | number;
  height?: string | number;
  radius?: string;
}) {
  return (
    <span
      className={styles.block}
      style={{ width, height, borderRadius: radius }}
      aria-hidden
    />
  );
}

/** A glass card filled with shimmering lines, mirroring a section's shape. */
export function GlassSkeleton({ lines = 4 }: { lines?: number }) {
  return (
    <GlassPanel className={styles.panel}>
      <div className={styles.header}>
        <SkeletonBlock width={38} height={38} radius="var(--radius-md)" />
        <div className={styles.headerTitles}>
          <SkeletonBlock width="60%" height="0.95rem" />
          <SkeletonBlock width="35%" height="0.7rem" />
        </div>
      </div>
      <div className={styles.list}>
        {Array.from({ length: lines }).map((_, i) => (
          <div className={styles.row} key={i}>
            <SkeletonBlock width={32} height={32} radius="var(--radius-sm)" />
            <div className={styles.rowTitles}>
              <SkeletonBlock width={`${55 + ((i * 7) % 35)}%`} height="0.85rem" />
              <SkeletonBlock width={`${30 + ((i * 5) % 20)}%`} height="0.7rem" />
            </div>
          </div>
        ))}
      </div>
    </GlassPanel>
  );
}
