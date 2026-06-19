import { Link } from "react-router-dom";
import { GlassPanel } from "../components/GlassPanel";
import { HomeIcon } from "../components/icons";
import styles from "./NotFound.module.css";

export function NotFound() {
  return (
    <div className={styles.page}>
      <GlassPanel variant="strong" className={styles.card}>
        <p className={styles.code}>404</p>
        <h1 className={styles.title}>Page not found</h1>
        <p className={styles.text}>
          The page you're looking for doesn't exist or hasn't been built yet.
        </p>
        <Link to="/" className={styles.link}>
          <HomeIcon className={styles.icon} />
          Back home
        </Link>
      </GlassPanel>
    </div>
  );
}
