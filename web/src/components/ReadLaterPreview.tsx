import { useState } from "react";
import type { ReadLaterItem } from "../lib/types";
import { READ_LATER } from "../features/home/mockData";
import { createReadLater } from "../lib/api/readLater";
import { ApiError } from "../lib/api/client";
import { GlassPanel } from "./GlassPanel";
import { SectionHeader } from "./SectionHeader";
import { BookmarkIcon, ClockIcon, HeartIcon, PlusIcon } from "./icons";
import { useI18n } from "../i18n/I18nProvider";
import styles from "./ReadLaterPreview.module.css";

export function ReadLaterPreview({
  items = READ_LATER,
  onSeeAll,
  onAdded,
}: {
  items?: ReadLaterItem[];
  onSeeAll?: () => void;
  onAdded?: () => void;
}) {
  const { t } = useI18n();
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const next = url.trim();
    if (!next) {
      setError(t("readlaterPreview.urlRequired"));
      return;
    }
    try {
      new URL(next);
    } catch {
      setError(t("readlaterPreview.urlInvalid"));
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await createReadLater({ url: next });
      setUrl("");
      onAdded?.();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("readlaterPreview.addFailed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <GlassPanel as="section" className={styles.panel}>
      <SectionHeader
        icon={<BookmarkIcon />}
        title={t("readlaterPreview.title")}
        actionLabel={t("readlaterPreview.seeAll")}
        onAction={onSeeAll}
      />

      <form className={styles.addForm} onSubmit={submit}>
        <input
          type="url"
          className={styles.addInput}
          placeholder={t("readlaterPreview.addPlaceholder")}
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          aria-label={t("readlaterPreview.addPlaceholder")}
        />
        <button type="submit" className={styles.addButton} disabled={busy}>
          <PlusIcon className={styles.addIcon} />
          <span>{busy ? t("readlaterPreview.adding") : t("readlaterPreview.add")}</span>
        </button>
      </form>
      {error ? <p className={styles.error} role="alert">{error}</p> : null}

      {items.length > 0 ? (
        <ul className={styles.list}>
          {items.map((item) => (
            <li key={item.id}>
              <a
                className={styles.item}
                href={item.url}
                target="_blank"
                rel="noreferrer noopener"
              >
                <div className={styles.body}>
                  <div className={styles.titleRow}>
                    {item.favorite ? (
                      <HeartIcon className={styles.heart} />
                    ) : null}
                    <span className={styles.title}>{item.title}</span>
                  </div>
                  <div className={styles.meta}>
                    <span className={styles.source}>{item.source ?? item.domain}</span>
                    <span className={styles.dot} aria-hidden>·</span>
                    <span className={styles.reading}>
                      <ClockIcon className={styles.clock} />
                      {item.readingTime} {t("card.min")}
                    </span>
                  </div>
                </div>
              </a>
            </li>
          ))}
        </ul>
      ) : (
        <p className={styles.empty}>{t("readlaterPreview.empty")}</p>
      )}
    </GlassPanel>
  );
}
