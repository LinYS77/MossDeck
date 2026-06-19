import { useState, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { GlassPanel } from "../components/GlassPanel";
import { Button } from "../components/ui";
import { Modal } from "../components/Modal";
import {
  DownloadIcon,
  UploadIcon,
  HomeIcon,
  SettingsIcon,
  CheckIcon,
  CloseIcon,
} from "../components/icons";
import { exportBackup, importBackup } from "../lib/api/backup";
import type { BackupExport, ImportSummary } from "../lib/api/backup";
import { useI18n, SUPPORTED_LOCALES } from "../i18n/I18nProvider";
import { ApiError } from "../lib/api/client";
import { useQuickAccessLimit, type QuickAccessLimit } from "../lib/deviceSettings";
import styles from "./SettingsPage.module.css";
import { cn } from "../lib/cn";

export function SettingsPage() {
  const navigate = useNavigate();
  const { t, locale, setLocale } = useI18n();
  const { limit: quickAccessLimit, setLimit: setQuickAccessLimit, options: quickAccessOptions } = useQuickAccessLimit();

  const [exportBusy, setExportBusy] = useState(false);
  const [importBusy, setImportBusy] = useState(false);
  const [mode, setMode] = useState<"merge" | "replace">("merge");
  const [summary, setSummary] = useState<ImportSummary | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showConfirm, setShowConfirm] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const pendingBackup = useRef<BackupExport | null>(null);

  const handleExport = async () => {
    setExportBusy(true);
    setError(null);
    try {
      const data = await exportBackup();
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      const date = new Date().toISOString().slice(0, 10).replace(/-/g, "");
      a.href = url;
      a.download = `homepage-backup-${date}.json`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("settings.exportFailed"));
    } finally {
      setExportBusy(false);
    }
  };

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setError(null);
    const reader = new FileReader();
    reader.onload = () => {
      try {
        const backup = JSON.parse(reader.result as string) as BackupExport;
        pendingBackup.current = backup;
        if (mode === "replace") {
          setShowConfirm(true);
        } else {
          doImport(backup, "merge");
        }
      } catch {
        setError(t("settings.invalidJSON"));
      }
    };
    reader.readAsText(file);
    // Reset so the same file can be re-selected.
    if (fileRef.current) fileRef.current.value = "";
  };

  const doImport = async (backup: BackupExport, m: "merge" | "replace") => {
    setShowConfirm(false);
    setImportBusy(true);
    setError(null);
    setSummary(null);
    try {
      const result = await importBackup({ mode: m, backup });
      setSummary(result);
      if (result.errors && result.errors.length > 0) {
        setError(result.errors.join("; "));
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("settings.importFailed"));
    } finally {
      setImportBusy(false);
    }
  };

  const confirmReplace = () => {
    if (pendingBackup.current) {
      doImport(pendingBackup.current, "replace");
    }
    pendingBackup.current = null;
  };

  const formatCount = (c: { created: number; updated: number; skipped: number }) =>
    `+${c.created} / ~${c.updated} / −${c.skipped}`;

  return (
    <div className={styles.page}>
      <main className={styles.main}>
        <GlassPanel variant="strong" className={styles.card}>
          <div className={styles.cardHead}>
            <span className={styles.cardIcon} aria-hidden>
              <SettingsIcon />
            </span>
            <h1 className={styles.cardTitle}>{t("settings.title")}</h1>
            <p className={styles.cardDesc}>{t("settings.description")}</p>
          </div>

          {/* ---- Language ---- */}
          <section className={styles.section}>
            <div className={styles.sectionHead}>
              <h2 className={styles.sectionTitle}>{t("settings.language")}</h2>
            </div>
            <div className={styles.langRow}>
              {SUPPORTED_LOCALES.map((l) => (
                <button
                  key={l}
                  type="button"
                  className={cn(styles.langBtn, locale === l && styles.langBtnActive)}
                  onClick={() => setLocale(l)}
                >
                  {l === "zh-CN" ? "中文" : "English"}
                </button>
              ))}
            </div>
          </section>

          {/* ---- Quick access count ---- */}
          <section className={styles.section}>
            <div className={styles.sectionHead}>
              <h2 className={styles.sectionTitle}>{t("settings.quickAccessTitle")}</h2>
              <p className={styles.sectionDesc}>{t("settings.quickAccessDesc")}</p>
            </div>
            <div className={styles.choiceGrid}>
              {quickAccessOptions.map((n) => (
                <button
                  key={n}
                  type="button"
                  className={cn(styles.choiceBtn, quickAccessLimit === n && styles.choiceBtnActive)}
                  onClick={() => setQuickAccessLimit(n as QuickAccessLimit)}
                  aria-pressed={quickAccessLimit === n}
                >
                  {n}
                </button>
              ))}
            </div>
          </section>

          {error ? (
            <div className={styles.banner} role="alert">
              {error}
            </div>
          ) : null}

          {/* ---- Export ---- */}
          <section className={styles.section}>
            <div className={styles.sectionHead}>
              <h2 className={styles.sectionTitle}>{t("settings.exportTitle")}</h2>
              <p className={styles.sectionDesc}>
                {t("settings.exportDesc")}
              </p>
            </div>
            <div className={styles.sectionActions}>
              <Button
                variant="primary"
                icon={<DownloadIcon />}
                loading={exportBusy}
                onClick={handleExport}
              >
                {t("settings.downloadBackup")}
              </Button>
            </div>
          </section>

          {/* ---- Import ---- */}
          <section className={styles.section}>
            <div className={styles.sectionHead}>
              <h2 className={styles.sectionTitle}>{t("settings.restoreTitle")}</h2>
              <p className={styles.sectionDesc}>
                {t("settings.restoreDesc")}
              </p>
            </div>

            <div className={styles.modeRow}>
              <button
                type="button"
                className={cn(styles.modeCard, mode === "merge" && styles.modeCardActive)}
                onClick={() => setMode("merge")}
                aria-pressed={mode === "merge"}
              >
                <strong>{t("settings.modeMerge")}</strong>
                <span>{t("settings.modeMergeDesc")}</span>
              </button>
              <button
                type="button"
                className={cn(styles.modeCard, mode === "replace" && styles.modeCardActive)}
                onClick={() => setMode("replace")}
                aria-pressed={mode === "replace"}
              >
                <strong>{t("settings.modeReplace")}</strong>
                <span>{t("settings.modeReplaceDesc")}</span>
                <em className={styles.warn}>{t("settings.modeReplaceWarn")}</em>
              </button>
            </div>

            <input
              ref={fileRef}
              type="file"
              accept=".json,application/json"
              className={styles.fileInput}
              onChange={handleFileChange}
            />
            <div className={styles.sectionActions}>
              <Button
                variant="primary"
                icon={<UploadIcon />}
                loading={importBusy}
                onClick={() => {
                  setError(null);
                  fileRef.current?.click();
                }}
              >
                {t("settings.selectFile")}
              </Button>
            </div>

            {summary ? (
              <div className={styles.summary}>
                <h3 className={styles.summaryTitle}>
                  <CheckIcon className={styles.summaryIcon} />
                  {t("settings.importComplete")}
                </h3>
                <table className={styles.summaryTable}>
                  <thead>
                    <tr>
                      <th>{t("settings.summaryEntity")}</th>
                      <th>{t("settings.summaryCounts")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr>
                      <td>{t("settings.entityCategories")}</td>
                      <td>{formatCount(summary.categories)}</td>
                    </tr>
                    <tr>
                      <td>{t("settings.entityTags")}</td>
                      <td>{formatCount(summary.tags)}</td>
                    </tr>
                    <tr>
                      <td>{t("settings.entityBookmarks")}</td>
                      <td>{formatCount(summary.bookmarks)}</td>
                    </tr>
                    <tr>
                      <td>{t("settings.entityReadLater")}</td>
                      <td>{formatCount(summary.readLaterItems)}</td>
                    </tr>
                  </tbody>
                </table>
                {summary.warnings && summary.warnings.length > 0 ? (
                  <div className={styles.warnings}>
                    <strong>{t("settings.warnings")}</strong>
                    <ul>
                      {summary.warnings.map((w, i) => (
                        <li key={i}>{w}</li>
                      ))}
                    </ul>
                  </div>
                ) : null}
              </div>
            ) : null}
          </section>

          <button
            type="button"
            className={styles.backHome}
            onClick={() => navigate("/")}
          >
            <HomeIcon className={styles.backHomeIcon} />
            {t("common.backHome")}
          </button>
        </GlassPanel>
      </main>

      {/* ---- Replace confirmation ---- */}
      <Modal
        open={showConfirm}
        onClose={() => {
          setShowConfirm(false);
          pendingBackup.current = null;
        }}
        title={t("settings.confirmTitle")}
        icon={<CloseIcon />}
      >
        <p className={styles.confirmText}>
          {t("settings.confirmText")}
        </p>
        <p className={styles.confirmText}>
          {t("settings.confirmHint")}
        </p>
        <div className={styles.confirmActions}>
          <Button variant="ghost" onClick={() => setShowConfirm(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="danger" onClick={confirmReplace}>
            {t("settings.replaceAll")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
