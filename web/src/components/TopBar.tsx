import { NavLink, useNavigate } from "react-router-dom";
import { SettingsIcon, LogoutIcon } from "./icons";
import { WallpaperSwitcher } from "./WallpaperSwitcher";
import { useAuth } from "../features/auth/AuthProvider";
import { useI18n } from "../i18n/I18nProvider";
import { cn } from "../lib/cn";
import styles from "./TopBar.module.css";

interface NavItem {
  to: string;
  zh: string;
  en: string;
  end?: boolean;
}

const NAV: NavItem[] = [
  { to: "/", zh: "首页", en: "Home", end: true },
  { to: "/bookmarks", zh: "书签", en: "Bookmarks" },
  { to: "/read-later", zh: "稍后读", en: "Read later" },
];

/** Neo-brutalist top bar: acid brand mark, bilingual pill nav, and
 *  wallpaper / settings / sign-out on the right. Transparent over the paper
 *  canvas — no solid bar, no backdrop jitter. */
export function TopBar() {
  const { logout } = useAuth();
  const navigate = useNavigate();
  const { t } = useI18n();

  return (
    <header className={styles.bar}>
      <NavLink to="/" className={styles.brand} aria-label="Moss Deck">
        <span className={styles.mark} aria-hidden>
          <img src="/brand/mossdeck.svg" alt="" className={styles.markImage} />
        </span>
        <span className={styles.brandText}>
          Moss
          <small>Deck</small>
        </span>
      </NavLink>

      <nav className={styles.nav} aria-label="Primary">
        {NAV.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.end}
            className={({ isActive }) => cn(styles.navLink, isActive && styles.navLinkActive)}
          >
            <span className={styles.duo}>
              <span>{item.zh}</span>
              <small>{item.en}</small>
            </span>
          </NavLink>
        ))}
      </nav>

      <div className={styles.actions}>
        <WallpaperSwitcher />
        <button
          type="button"
          className={styles.iconBtn}
          onClick={() => navigate("/settings")}
          title={t("topbar.settings")}
          aria-label={t("topbar.settings")}
        >
          <SettingsIcon />
        </button>
        <button
          type="button"
          className={styles.iconBtn}
          onClick={() => void logout()}
          title={t("topbar.signOut")}
          aria-label={t("topbar.signOut")}
        >
          <LogoutIcon />
        </button>
      </div>
    </header>
  );
}
