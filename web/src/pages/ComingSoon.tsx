import { Link } from "react-router-dom";
import { GlassPanel } from "../components/GlassPanel";
import { HomeIcon } from "../components/icons";
import notFoundStyles from "./NotFound.module.css";
import styles from "./ComingSoon.module.css";

interface ComingSoonProps {
  title: string;
  description?: string;
}

/** Placeholder for routes reserved in the nav but not yet implemented. */
export function ComingSoon({ title, description }: ComingSoonProps) {
  return (
    <div className={notFoundStyles.page}>
      <GlassPanel variant="strong" className={`${notFoundStyles.card} ${styles.card}`}>
        <p className={`accent-text ${styles.eyebrow}`}>Coming soon</p>
        <h1 className={notFoundStyles.title}>{title}</h1>
        <p className={notFoundStyles.text}>
          {description ??
            "This view is scaffolded but not wired to the API yet. It will land in a follow-up."}
        </p>
        <Link to="/" className={notFoundStyles.link}>
          <HomeIcon className={notFoundStyles.icon} />
          Back home
        </Link>
      </GlassPanel>
    </div>
  );
}
