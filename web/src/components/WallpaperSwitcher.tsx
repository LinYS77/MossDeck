import { useEffect, useId, useRef, useState } from "react";
import { WALLPAPERS, useWallpaper, useWallpaperEnabled } from "../lib/wallpapers";
import { cn } from "../lib/cn";
import { ImageIcon, CheckIcon } from "./icons";
import { useI18n } from "../i18n/I18nProvider";
import styles from "./WallpaperSwitcher.module.css";

/** Glass dropdown that toggles the optional decorative wallpaper layer and,
 *  when enabled, previews / switches the active wallpaper. */
export function WallpaperSwitcher() {
  const { wallpaper, setSlug } = useWallpaper();
  const { enabled, setEnabled } = useWallpaperEnabled();
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const menuId = useId();
  const wrapRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!wrapRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div className={styles.wrap} ref={wrapRef}>
      <button
        type="button"
        className={styles.trigger}
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-controls={menuId}
        aria-pressed={enabled}
        title={t("wallpaper.change")}
      >
        <ImageIcon className={styles.triggerIcon} />
        <span className={styles.triggerLabel}>{t("wallpaper.label")}</span>
      </button>

      {open ? (
        <div className={styles.menu} id={menuId} role="dialog" aria-label={t("wallpaper.choose")}>
          <p className={styles.menuTitle}>{t("wallpaper.title")}</p>

          <button
            type="button"
            className={cn(styles.enableRow, enabled && styles.enableRowOn)}
            onClick={() => setEnabled(!enabled)}
            aria-pressed={enabled}
          >
            <span className={styles.enableLabel}>
              壁纸背景
              <small>Wallpaper layer</small>
            </span>
            <span className={styles.toggle} aria-hidden>
              <span className={styles.toggleKnob} />
            </span>
          </button>

          <div className={styles.grid}>
            {WALLPAPERS.map((w) => {
              const active = enabled && w.slug === wallpaper.slug;
              return (
                <button
                  key={w.slug}
                  type="button"
                  className={cn(styles.cell, active && styles.cellActive)}
                  onClick={() => {
                    if (!enabled) setEnabled(true);
                    setSlug(w.slug);
                  }}
                  title={w.label}
                  aria-pressed={active}
                >
                  <img
                    src={w.thumb}
                    alt=""
                    className={styles.thumb}
                    loading="lazy"
                    decoding="async"
                  />
                  <span className={styles.cellLabel}>{w.label}</span>
                  {active ? (
                    <span className={styles.check}>
                      <CheckIcon className={styles.checkIcon} />
                    </span>
                  ) : null}
                </button>
              );
            })}
          </div>
        </div>
      ) : null}
    </div>
  );
}
