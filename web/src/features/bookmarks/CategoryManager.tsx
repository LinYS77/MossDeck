import { useEffect, useState } from "react";
import type {
  CategoryDTO,
  CreateCategoryParams,
  UpdateCategoryParams,
} from "../../lib/api/bookmarks";
import {
  Button,
  IconButton,
  TextInput,
} from "../../components/ui";
import { EditIcon, TrashIcon, HomeIcon } from "../../components/icons";
import { useI18n } from "../../i18n/I18nProvider";
import styles from "./CategoryManager.module.css";

interface CategoryManagerProps {
  categories: CategoryDTO[];
  busy: boolean;
  onCreate: (p: CreateCategoryParams) => Promise<unknown>;
  onUpdate: (id: number, p: UpdateCategoryParams) => Promise<unknown>;
  onDelete: (id: number) => Promise<unknown>;
}

/** Inline CRUD list for categories. Lives inside the management modal. */
export function CategoryManager({
  categories,
  busy,
  onCreate,
  onUpdate,
  onDelete,
}: CategoryManagerProps) {
  const { t } = useI18n();
  const [name, setName] = useState("");
  const [editing, setEditing] = useState<Record<number, string>>({});

  useEffect(() => {
    // Initialise edit buffers for existing names.
    setEditing((prev) => {
      const next: Record<number, string> = {};
      for (const c of categories) next[c.id] = prev[c.id] ?? c.name;
      return next;
    });
  }, [categories]);

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
          placeholder={t("categoryManager.newPlaceholder")}
          value={name}
          onChange={(e) => setName(e.target.value)}
          aria-label={t("categoryManager.newPlaceholder")}
        />
        <Button
          type="submit"
          variant="primary"
          size="sm"
          className={styles.addButton}
          disabled={busy || !name.trim()}
        >
          {t("categoryManager.add")}
        </Button>
      </form>

      {categories.length === 0 ? (
        <p className={styles.empty}>{t("categoryManager.empty")}</p>
      ) : (
        <ul className={styles.list}>
          {categories.map((c) => (
            <li key={c.id} className={styles.item}>
              <span className={styles.dot} aria-hidden />
              <input
                className={styles.nameInput}
                value={editing[c.id] ?? c.name}
                onChange={(e) =>
                  setEditing((prev) => ({ ...prev, [c.id]: e.target.value }))
                }
                aria-label={`${t("bookmarkForm.category")} ${c.name}`}
              />
              <span className={styles.count}>{c.bookmarkCount}</span>
              <IconButton
                label={
                  c.showOnHome
                    ? t("categoryManager.hideOnHome")
                    : t("categoryManager.showOnHome")
                }
                active={c.showOnHome}
                onClick={() => onUpdate(c.id, { showOnHome: !c.showOnHome })}
              >
                <HomeIcon />
              </IconButton>
              <IconButton
                label={t("categoryManager.saveLabel")}
                onClick={() =>
                  onUpdate(c.id, { name: (editing[c.id] ?? c.name).trim() })
                }
              >
                <EditIcon />
              </IconButton>
              <IconButton
                label={c.archived ? t("categoryManager.archivedLabel") : t("categoryManager.archiveLabel")}
                variant="danger"
                onClick={() => onDelete(c.id)}
              >
                <TrashIcon />
              </IconButton>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
