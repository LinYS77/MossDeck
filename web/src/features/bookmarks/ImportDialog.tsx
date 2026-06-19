import { useRef, useState } from "react";
import { Modal } from "../../components/Modal";
import { Button, FormField } from "../../components/ui";
import {
  UploadIcon,
  CheckIcon,
} from "../../components/icons";
import {
  importBookmarksHTML,
  type DuplicateMode,
  type ImportResultDTO,
} from "../../lib/api/bookmarks";
import { cn } from "../../lib/cn";
import { useI18n } from "../../i18n/I18nProvider";
import styles from "./ImportDialog.module.css";

interface ImportDialogProps {
  open: boolean;
  onClose: () => void;
  onImported: () => void;
}

const MODES: { value: DuplicateMode; labelKey: string; hintKey: string }[] = [
  { value: "skip", labelKey: "import.modeSkip", hintKey: "import.modeSkipHint" },
  { value: "update", labelKey: "import.modeUpdate", hintKey: "import.modeUpdateHint" },
  { value: "duplicate", labelKey: "import.modeDuplicate", hintKey: "import.modeDuplicateHint" },
];

function getModeLabel(mode: string, t: (k: string) => string): string {
  switch (mode) {
    case "skip": return t("filters.modeSkip");
    case "update": return t("filters.modeUpdate");
    case "duplicate": return t("filters.modeDuplicate");
    default: return mode;
  }
}

/** Browser-bookmarks HTML import. Upload a file or paste HTML, choose a
 *  duplicate policy, and see a detailed result summary with sample reasons. */
export function ImportDialog({ open, onClose, onImported }: ImportDialogProps) {
  const { t } = useI18n();
  const [file, setFile] = useState<File | null>(null);
  const [pasted, setPasted] = useState("");
  const [mode, setMode] = useState<DuplicateMode>("skip");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ImportResultDTO | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const reset = () => {
    setFile(null);
    setPasted("");
    setResult(null);
    setError(null);
  };

  const close = () => {
    reset();
    onClose();
  };

  const submit = async () => {
    setBusy(true);
    setError(null);
    setResult(null);
    try {
      const payload = file ?? (pasted.trim() ? pasted.trim() : null);
      if (!payload) {
        setError(t("import.errorChooseFile"));
        setBusy(false);
        return;
      }
      const res = await importBookmarksHTML(payload, mode);
      setResult(res);
      onImported();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("import.errorFallback"));
    } finally {
      setBusy(false);
    }
  };

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragOver(false);
    const f = e.dataTransfer.files?.[0];
    if (f) {
      setFile(f);
      setPasted("");
      setResult(null);
    }
  };

  const hasInput = !!file || !!pasted.trim();

  return (
    <Modal
      open={open}
      onClose={close}
      title={t("import.title")}
      description={t("import.description")}
      icon={<UploadIcon />}
      footer={
        result ? (
          <Button variant="primary" icon={<CheckIcon />} onClick={close}>
            {t("common.done")}
          </Button>
        ) : (
          <>
            <Button variant="ghost" onClick={close} disabled={busy}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="primary"
              icon={<UploadIcon />}
              onClick={submit}
              loading={busy}
              disabled={!hasInput}
            >
              {busy ? t("import.importing") : t("import.importBtn")}
            </Button>
          </>
        )
      }
    >
      {result ? (
        <ImportResult result={result} />
      ) : (
        <div className={styles.body}>
          {/* Drop zone / file picker */}
          <div
            className={cn(styles.dropzone, dragOver && styles.dropzoneOver)}
            onDragOver={(e) => {
              e.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={onDrop}
            onClick={() => inputRef.current?.click()}
            role="button"
            tabIndex={0}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") inputRef.current?.click();
            }}
          >
            <input
              ref={inputRef}
              type="file"
              accept=".html,.htm,text/html"
              className={styles.fileInput}
              onChange={(e) => {
                const f = e.target.files?.[0] ?? null;
                setFile(f);
                setPasted("");
                setResult(null);
              }}
            />
            <UploadIcon className={styles.dropzoneIcon} />
            {file ? (
              <div className={styles.fileInfo}>
                <strong>{file.name}</strong>
                <span>{(file.size / 1024).toFixed(0)} KB</span>
              </div>
            ) : (
              <div className={styles.dropzoneText}>
                <strong>{t("import.dropzoneClick")}</strong> {t("import.dropzoneDrag")}
                <span className={styles.muted}>{t("import.dropzoneMaxSize")}</span>
              </div>
            )}
          </div>

          <div className={styles.or}>{t("import.orPasteHTML")}</div>

          <FormField label={t("import.pasteFieldLabel")}>
            <textarea
              className={styles.paste}
              placeholder="<DL><p>…your Netscape export…</p></DL>"
              value={pasted}
              rows={4}
              onChange={(e) => {
                setPasted(e.target.value);
                setFile(null);
                setResult(null);
              }}
            />
          </FormField>

          <FormField label={t("import.duplicatePolicyLabel")}>
            <div className={styles.modes}>
              {MODES.map((m) => (
                <button
                  key={m.value}
                  type="button"
                  className={styles.mode}
                  data-on={mode === m.value}
                  onClick={() => setMode(m.value)}
                >
                  <span className={styles.modeLabel}>{t(m.labelKey)}</span>
                  <span className={styles.modeHint}>{t(m.hintKey)}</span>
                </button>
              ))}
            </div>
          </FormField>

          {error ? <p className={styles.error} role="alert">{error}</p> : null}
        </div>
      )}
    </Modal>
  );
}

/** Result summary with counters and bounded sample reason lines. */
function ImportResult({ result }: { result: ImportResultDTO }) {
  const { t } = useI18n();
  const stats = [
    { labelKey: "import.statusCreated", value: result.created, tone: "good" as const },
    { labelKey: "import.statusUpdated", value: result.updated, tone: "neutral" as const },
    { labelKey: "import.statusSkipped", value: result.skipped, tone: "muted" as const },
    { labelKey: "import.statusFailed", value: result.failed, tone: "bad" as const },
  ];
  return (
    <div className={styles.result}>
      <div className={styles.resultHead}>
        <div>
          <p className={styles.resultTitle}>{t("import.resultTitle")}</p>
          <p className={styles.resultSub}>
            {t("import.resultParsed", { total: result.total, processed: result.processed })}
            {result.limitReached ? t("import.resultLimitReached") : ""}
          </p>
        </div>
        <span className={styles.modeBadge}>{getModeLabel(result.duplicateMode, t)}</span>
      </div>

      <div className={styles.stats}>
        {stats.map((s) => (
          <div key={t(s.labelKey)} className={styles.stat} data-tone={s.tone}>
            <span className={styles.statValue}>{s.value}</span>
            <span className={styles.statLabel}>{t(s.labelKey)}</span>
          </div>
        ))}
      </div>

      {result.samples && result.samples.length > 0 ? (
        <div className={styles.samples}>
          <p className={styles.samplesTitle}>
            {t("import.sampleReasonsTitle")}
            <span className={styles.samplesCount}>
              {t("import.sampleReasonsShowing", { count: result.samples.length })}
            </span>
          </p>
          <ul className={styles.sampleList}>
            {result.samples.map((s, i) => (
              <li key={i} className={styles.sample} data-kind={s.kind}>
                <span className={styles.sampleKind}>{s.kind === "skipped" ? t("import.statusSkipped") : s.kind === "failed" ? t("import.statusFailed") : s.kind}</span>
                <span className={styles.sampleBody}>
                  {s.url ? <span className={styles.sampleUrl}>{s.url}</span> : null}
                  <span className={styles.sampleReason}>{s.reason}</span>
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}
