import { useEffect, useState } from "react";
import type { ReadLaterDTO } from "../../lib/api/readLater";
import type { TagDTO } from "../../lib/api/bookmarks";
import { Button, FormField, TextArea, TextInput, CheckChip } from "../../components/ui";
import { StarIcon } from "../../components/icons";
import { useI18n } from "../../i18n/I18nProvider";
import styles from "./ReadLaterForm.module.css";

export interface ReadLaterFormValue {
  url: string;
  title: string;
  excerpt: string;
  author: string;
  siteName: string;
  source: string;
  readingTimeMinutes: number;
  priority: number;
  favorite: boolean;
  tagIds: number[];
}

export function readLaterToForm(item: ReadLaterDTO): ReadLaterFormValue {
  return {
    url: item.url,
    title: item.title,
    excerpt: item.excerpt ?? "",
    author: item.author ?? "",
    siteName: item.siteName ?? "",
    source: item.source ?? "",
    readingTimeMinutes: item.readingTimeMinutes ?? 0,
    priority: item.priority ?? 0,
    favorite: item.favorite,
    tagIds: (item.tags ?? []).map((t) => t.id),
  };
}

export const EMPTY_READ_LATER_FORM: ReadLaterFormValue = {
  url: "",
  title: "",
  excerpt: "",
  author: "",
  siteName: "",
  source: "",
  readingTimeMinutes: 0,
  priority: 0,
  favorite: false,
  tagIds: [],
};

interface ReadLaterFormProps {
  initial: ReadLaterFormValue;
  tags: TagDTO[];
  submitting: boolean;
  /** Server-side duplicate-URL error (409) shown inline on the URL field. */
  duplicateError?: string | null;
  onSubmit: (value: ReadLaterFormValue) => void;
  onCancel: () => void;
}

/** Create/edit form for a read-later item. Lives inside a Modal. */
export function ReadLaterForm({
  initial,
  tags,
  submitting,
  duplicateError,
  onSubmit,
  onCancel,
}: ReadLaterFormProps) {
  const [value, setValue] = useState<ReadLaterFormValue>(initial);
  const [urlError, setUrlError] = useState<string | null>(null);
  const { t } = useI18n();

  // Reset local state when the modal opens with a different item.
  useEffect(() => {
    setValue(initial);
    setUrlError(null);
  }, [initial]);

  const isEdit = initial.url !== "";

  const toggleTag = (id: number) => {
    setValue((v) => ({
      ...v,
      tagIds: v.tagIds.includes(id)
        ? v.tagIds.filter((t) => t !== id)
        : [...v.tagIds, id],
    }));
  };

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const url = value.url.trim();
    if (!url) {
      setUrlError(t("readlaterForm.urlRequired"));
      return;
    }
    try {
      new URL(url);
    } catch {
      setUrlError(t("readlaterForm.urlInvalid"));
      return;
    }
    setUrlError(null);
    onSubmit({ ...value, url });
  };

  // The URL field surfaces either client validation or the server 409.
  const shownUrlError = urlError ?? duplicateError ?? null;

  return (
    <form className={styles.form} onSubmit={submit}>
      <FormField label={t("readlaterForm.url")} required error={shownUrlError}>
        <TextInput
          type="url"
          placeholder="https://example.com/article"
          value={value.url}
          invalid={!!shownUrlError}
          onChange={(e) => setValue((v) => ({ ...v, url: e.target.value }))}
          autoFocus={!isEdit}
        />
      </FormField>

      <FormField label={t("readlaterForm.title")} hint={t("readlaterForm.titleHint")}>
        <TextInput
          placeholder="A memorable title"
          value={value.title}
          onChange={(e) => setValue((v) => ({ ...v, title: e.target.value }))}
        />
      </FormField>

      <FormField label={t("readlaterForm.excerpt")}>
        <TextArea
          placeholder={t("readlaterForm.excerptPlaceholder")}
          value={value.excerpt}
          onChange={(e) => setValue((v) => ({ ...v, excerpt: e.target.value }))}
        />
      </FormField>

      <div className={styles.row}>
        <FormField label={t("readlaterForm.author")} className={styles.grow}>
          <TextInput
            placeholder={t("readlaterForm.optional")}
            value={value.author}
            onChange={(e) => setValue((v) => ({ ...v, author: e.target.value }))}
          />
        </FormField>
        <FormField label={t("readlaterForm.siteName")} className={styles.grow}>
          <TextInput
            placeholder={t("readlaterForm.optional")}
            value={value.siteName}
            onChange={(e) => setValue((v) => ({ ...v, siteName: e.target.value }))}
          />
        </FormField>
      </div>

      <FormField label={t("readlaterForm.source")} hint={t("readlaterForm.sourceHint")}>
        <TextInput
          placeholder={t("readlaterForm.optional")}
          value={value.source}
          onChange={(e) => setValue((v) => ({ ...v, source: e.target.value }))}
        />
      </FormField>

      <div className={styles.row}>
        <FormField
          label={t("readlaterForm.readingTime")}
          className={styles.grow}
          hint={t("readlaterForm.readingTimeHint")}
        >
          <TextInput
            type="number"
            min={0}
            inputMode="numeric"
            value={Number.isFinite(value.readingTimeMinutes) ? value.readingTimeMinutes : 0}
            onChange={(e) =>
              setValue((v) => ({
                ...v,
                readingTimeMinutes: Math.max(0, Math.floor(Number(e.target.value) || 0)),
              }))
            }
          />
        </FormField>
        <FormField
          label={t("readlaterForm.priority")}
          className={styles.grow}
          hint={t("readlaterForm.priorityHint")}
        >
          <TextInput
            type="number"
            min={0}
            inputMode="numeric"
            value={Number.isFinite(value.priority) ? value.priority : 0}
            onChange={(e) =>
              setValue((v) => ({
                ...v,
                priority: Math.max(0, Math.floor(Number(e.target.value) || 0)),
              }))
            }
          />
        </FormField>
      </div>

      {tags.length > 0 ? (
        <FormField label={t("readlaterForm.tags")}>
          <div className={styles.tagGrid}>
            {tags.map((t) => {
              const on = value.tagIds.includes(t.id);
              return (
                <button
                  key={t.id}
                  type="button"
                  className={styles.tagChip}
                  data-on={on}
                  style={t.color ? { ["--tag" as string]: t.color } : undefined}
                  onClick={() => toggleTag(t.id)}
                  aria-pressed={on}
                >
                  {t.name}
                </button>
              );
            })}
          </div>
          {value.tagIds.length > 0 ? (
            <span className={styles.tagHint}>{t("readlaterForm.selected", { count: value.tagIds.length })}</span>
          ) : null}
        </FormField>
      ) : null}

      <div className={styles.toggles}>
        <CheckChip
          checked={value.favorite}
          onChange={(c) => setValue((v) => ({ ...v, favorite: c }))}
          label={t("readlaterForm.favorite")}
          icon={<StarIcon />}
        />
      </div>

      <div className={styles.actions}>
        <Button variant="ghost" onClick={onCancel} type="button">
          {t("readlaterForm.cancel")}
        </Button>
        <Button variant="primary" type="submit" loading={submitting}>
          {isEdit ? t("readlaterForm.saveChanges") : t("readlaterForm.addToQueue")}
        </Button>
      </div>
    </form>
  );
}
