import { useEffect, useState } from "react";
import type {
  BookmarkDTO,
  CategoryDTO,
  TagDTO,
} from "../../lib/api/bookmarks";
import { Button, FormField, Select, TextArea, TextInput, CheckChip } from "../../components/ui";
import { PinIcon, StarIcon } from "../../components/icons";
import { useI18n } from "../../i18n/I18nProvider";
import styles from "./BookmarkForm.module.css";

export interface BookmarkFormValue {
  url: string;
  title: string;
  description: string;
  categoryId: number;
  tagIds: number[];
  pinned: boolean;
  favorite: boolean;
}

export function bookmarkToForm(b: BookmarkDTO): BookmarkFormValue {
  return {
    url: b.url,
    title: b.title,
    description: b.description ?? "",
    categoryId: b.categoryId ?? 0,
    tagIds: (b.tags ?? []).map((t) => t.id),
    pinned: b.pinned,
    favorite: b.favorite,
  };
}

export const EMPTY_BOOKMARK_FORM: BookmarkFormValue = {
  url: "",
  title: "",
  description: "",
  categoryId: 0,
  tagIds: [],
  pinned: false,
  favorite: false,
};

interface BookmarkFormProps {
  initial: BookmarkFormValue;
  categories: CategoryDTO[];
  tags: TagDTO[];
  submitting: boolean;
  onSubmit: (value: BookmarkFormValue) => void;
  onCancel: () => void;
}

/** Create/edit form for a single bookmark. Lives inside a Modal. */
export function BookmarkForm({
  initial,
  categories,
  tags,
  submitting,
  onSubmit,
  onCancel,
}: BookmarkFormProps) {
  const [value, setValue] = useState<BookmarkFormValue>(initial);
  const [urlError, setUrlError] = useState<string | null>(null);
  const { t } = useI18n();

  // Reset local state when the modal opens with a different bookmark.
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
      setUrlError(t("bookmarkForm.urlRequired"));
      return;
    }
    try {
      // Basic validation — the backend normalizes too.
      new URL(url);
    } catch {
      setUrlError(t("bookmarkForm.urlInvalid"));
      return;
    }
    setUrlError(null);
    onSubmit({ ...value, url });
  };

  return (
    <form className={styles.form} onSubmit={submit}>
      <FormField label={t("bookmarkForm.url")} required error={urlError}>
        <TextInput
          type="url"
          placeholder="https://example.com"
          value={value.url}
          invalid={!!urlError}
          onChange={(e) => setValue((v) => ({ ...v, url: e.target.value }))}
          autoFocus={!isEdit}
        />
      </FormField>

      <FormField label={t("bookmarkForm.title")} hint={t("bookmarkForm.titleHint")}>
        <TextInput
          placeholder="A memorable title"
          value={value.title}
          onChange={(e) => setValue((v) => ({ ...v, title: e.target.value }))}
        />
      </FormField>

      <FormField label={t("bookmarkForm.description")}>
        <TextArea
          placeholder={t("bookmarkForm.descriptionPlaceholder")}
          value={value.description}
          onChange={(e) => setValue((v) => ({ ...v, description: e.target.value }))}
        />
      </FormField>

      <div className={styles.row}>
        <FormField label={t("bookmarkForm.category")} className={styles.grow}>
          <Select
            value={value.categoryId}
            onChange={(e) =>
              setValue((v) => ({ ...v, categoryId: Number(e.target.value) }))
            }
          >
            <option value={0}>{t("bookmarkForm.uncategorized")}</option>
            {categories
              .filter((c) => !c.archived)
              .map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
          </Select>
        </FormField>
      </div>

      {tags.length > 0 ? (
        <FormField label={t("bookmarkForm.tags")}>
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
            <span className={styles.tagHint}>
              {t("bookmarkForm.selected", { count: value.tagIds.length })}
            </span>
          ) : null}
        </FormField>
      ) : null}

      <div className={styles.toggles}>
        <CheckChip
          checked={value.pinned}
          onChange={(c) => setValue((v) => ({ ...v, pinned: c }))}
          label={t("bookmarkForm.pinned")}
          icon={<PinIcon />}
        />
        <CheckChip
          checked={value.favorite}
          onChange={(c) => setValue((v) => ({ ...v, favorite: c }))}
          label={t("bookmarkForm.favorite")}
          icon={<StarIcon />}
        />
      </div>

      <div className={styles.actions}>
        <Button variant="ghost" onClick={onCancel} type="button">
          {t("bookmarkForm.cancel")}
        </Button>
        <Button variant="primary" type="submit" loading={submitting}>
          {isEdit ? t("bookmarkForm.saveChanges") : t("bookmarkForm.addBookmark")}
        </Button>
      </div>
    </form>
  );
}
