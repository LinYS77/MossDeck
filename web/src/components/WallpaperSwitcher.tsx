import { useEffect, useId, useRef, useState } from "react";
import { createCustomWallpaper, deleteCustomWallpaper, useWallpaper, useWallpaperEnabled } from "../lib/wallpapers";
import { cn } from "../lib/cn";
import { ImageIcon, CheckIcon, UploadIcon } from "./icons";
import { useI18n } from "../i18n/I18nProvider";
import styles from "./WallpaperSwitcher.module.css";

/** Glass dropdown that toggles the optional decorative wallpaper layer and,
 *  when enabled, previews / switches the active wallpaper. */
export function WallpaperSwitcher() {
  const { wallpaper, wallpapers, setSlug } = useWallpaper();
  const { enabled, setEnabled } = useWallpaperEnabled();
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const menuId = useId();
  const wrapRef = useRef<HTMLDivElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);

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

  const handleUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;
    setUploading(true);
    try {
      const custom = await createCustomWallpaper(file);
      setSlug(custom.slug);
      setEnabled(true);
    } finally {
      setUploading(false);
      event.target.value = "";
    }
  };

  const handleDelete = (slug: string) => {
    const active = wallpaper?.slug === slug;
    const next = deleteCustomWallpaper(slug);
    if (active) {
      if (next[0]) {
        setSlug(next[0].slug);
      } else {
        setSlug("");
        setEnabled(false);
      }
    }
  };

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
            onClick={() => {
              if (enabled || wallpapers.length > 0) setEnabled(!enabled);
            }}
            aria-pressed={enabled}
            disabled={!enabled && wallpapers.length === 0}
          >
            <span className={styles.enableLabel}>
              壁纸背景
              <small>Wallpaper layer</small>
            </span>
            <span className={styles.toggle} aria-hidden>
              <span className={styles.toggleKnob} />
            </span>
          </button>

          <input
            ref={fileRef}
            type="file"
            accept="image/*"
            className={styles.fileInput}
            onChange={handleUpload}
          />
          <button
            type="button"
            className={styles.uploadRow}
            onClick={() => fileRef.current?.click()}
            disabled={uploading}
          >
            <UploadIcon className={styles.uploadIcon} />
            <span>
              {t("wallpaper.upload")}
              <small>{t("wallpaper.uploadHint")}</small>
            </span>
          </button>

          <div className={styles.grid}>
            {wallpapers.length > 0 ? wallpapers.map((w) => {
              const active = enabled && wallpaper?.slug === w.slug;
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
                  <span
                    role="button"
                    tabIndex={0}
                    className={styles.deleteBtn}
                    aria-label={t("wallpaper.delete")}
                    title={t("wallpaper.delete")}
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDelete(w.slug);
                    }}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        e.stopPropagation();
                        handleDelete(w.slug);
                      }
                    }}
                  >
                    ×
                  </span>
                </button>
              );
            }) : (
              <p className={styles.empty}>{t("wallpaper.empty")}</p>
            )}
          </div>
        </div>
      ) : null}
    </div>
  );
}
