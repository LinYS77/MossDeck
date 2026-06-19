import { GlassPanel } from "./GlassPanel";
import { CloudSunIcon, NoteIcon } from "./icons";
import styles from "./Widgets.module.css";

/** Weather placeholder. Real implementation will hit a geo/weather API. */
export function WeatherWidget() {
  return (
    <GlassPanel as="section" className={styles.panel}>
      <div className={styles.weatherTop}>
        <CloudSunIcon className={styles.weatherIcon} />
        <div>
          <div className={styles.temp}>
            21<span className={styles.deg}>°</span>
          </div>
          <div className={styles.cond}>Partly cloudy</div>
        </div>
      </div>
      <div className={styles.weatherFoot}>
        <span>Shanghai</span>
        <span className={styles.spacer}>·</span>
        <span>H 24° L 16°</span>
      </div>
    </GlassPanel>
  );
}

/** Notes placeholder. Real implementation will be a quick scratchpad widget. */
export function NotesWidget() {
  return (
    <GlassPanel as="section" className={styles.panel}>
      <div className={styles.notesHead}>
        <NoteIcon className={styles.notesIcon} />
        <span className={styles.notesTitle}>Notes</span>
      </div>
      <p className={styles.notesBody}>
        Welcome home. This glass panel is a placeholder for a quick scratchpad —
        wire it to the API later.
      </p>
    </GlassPanel>
  );
}
