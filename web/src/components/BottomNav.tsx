import { NavLink } from "react-router-dom";
import { HomeIcon, LayersIcon, BookmarkIcon, SettingsIcon } from "./icons";
import { cn } from "../lib/cn";
import styles from "./BottomNav.module.css";

const items = [
  { to: "/", label: "Home", icon: HomeIcon, end: true },
  { to: "/bookmarks", label: "Bookmarks", icon: LayersIcon, end: false },
  { to: "/read-later", label: "Read later", icon: BookmarkIcon, end: false },
  { to: "/settings", label: "Settings", icon: SettingsIcon, end: false },
];

/** Mobile-only glass bottom navigation. Hidden on >= 721px widths. */
export function BottomNav() {
  return (
    <nav className={styles.nav} aria-label="Primary">
      {items.map(({ to, label, icon: Icon, end }) => (
        <NavLink
          key={to}
          to={to}
          end={end}
          className={({ isActive }) => cn(styles.item, isActive && styles.itemActive)}
        >
          <Icon className={styles.icon} />
          <span className={styles.label}>{label}</span>
        </NavLink>
      ))}
    </nav>
  );
}
