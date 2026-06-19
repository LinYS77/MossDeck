import { useEffect, useState } from "react";
import type {
  CreateTagParams,
  TagDTO,
  UpdateTagParams,
} from "../../lib/api/bookmarks";
import { Button, TextInput } from "../../components/ui";
import { EditIcon, TrashIcon } from "../../components/icons";
import { useI18n } from "../../i18n/I18nProvider";
import styles from "./TagManager.module.css";

interface TagManagerProps {
  tags: TagDTO[];
  busy: boolean;
  onCreate: (p: CreateTagParams) => Promise<unknown>;
  onUpdate: (id: number, p: UpdateTagParams) => Promise<unknown>;
  onDelete: (id: number) => Promise<unknown>;
}

/** Inline CRUD list for tags. Mirrors the CategoryManager layout. */
export function TagManager({ tags, busy, onCreate, onUpdate, onDelete }: TagManagerProps) {
  const { t } = useI18n();
  const [name, setName] = useState("");
  const [editing, setEditing] = useState<Record<number, string>>({});

  useEffect(() => {
    setEditing((prev) => {
      const next: Record<number, string> = {};
      for (const t of tags) next[t.id] = prev[t.id] ?? t.name;
      return next;
    });
  }, [tags]);

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    await onCreate({ name: trimmed });
    setName("");
  };

  return (
    <div className={styles.wrap}>
      <form className={styles.addRow} onSubmit={add}>
        <TextInput
          placeholder={t("tagManager.newPlaceholder")}
          value={name}
          onChange={(e) => setName(e.target.value)}
          aria-label={t("tagManager.newPlaceholder")}
        />
        <Button
          type="submit"
          variant="primary"
          size="sm"
          className={styles.addButton}
          disabled={busy || !name.trim()}
        >
          {t("tagManager.add")}
        </Button>
      </form>

      {tags.length === 0 ? (
        <p className={styles.empty}>{t("tagManager.empty")}</p>
      ) : (
        <div className={styles.chips}>
          {tags.map((tag) => (
            <span
              key={tag.id}
              className={styles.chip}
              style={tag.color ? { ["--tag" as string]: tag.color } : undefined}
            >
              <input
                className={styles.nameInput}
                value={editing[tag.id] ?? tag.name}
                onChange={(e) =>
                  setEditing((prev) => ({ ...prev, [tag.id]: e.target.value }))
                }
                aria-label={`${t("tagManager.tagNameLabel")} ${tag.name}`}
              />
              <button
                type="button"
                className={styles.chipBtn}
                aria-label={t("tagManager.saveLabel")}
                onClick={() => onUpdate(tag.id, { name: (editing[tag.id] ?? tag.name).trim() })}
              >
                <EditIcon />
              </button>
              <button
                type="button"
                className={styles.chipBtn}
                aria-label={t("tagManager.deleteLabel")}
                onClick={() => onDelete(tag.id)}
              >
                <TrashIcon />
              </button>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
