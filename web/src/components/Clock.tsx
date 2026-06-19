import { useClock } from "../lib/useClock";
import styles from "./Clock.module.css";

interface ClockProps {
  /** Name shown in the greeting (e.g. the signed-in user's display name). */
  name?: string;
}

/** Hero greeting + live clock. The visual anchor above the search bar. */
export function Clock({ name }: ClockProps) {
  const { greeting, time, seconds, dateLabel } = useClock();

  return (
    <div className={styles.wrap}>
      <p className={styles.greeting}>
        {greeting}
        {name ? (
          <>
            , <span className="accent-text">{name}</span>
          </>
        ) : null}
      </p>
      <div className={styles.timeRow}>
        <span className={`tabular ${styles.time}`}>{time}</span>
        <span className={`tabular ${styles.seconds}`}>{seconds}</span>
      </div>
      <p className={styles.date}>{dateLabel}</p>
    </div>
  );
}
