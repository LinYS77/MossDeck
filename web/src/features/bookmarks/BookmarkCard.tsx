import type { BookmarkDTO } from "../../lib/api/bookmarks";
import { IconButton } from "../../components/ui";
import {
  EditIcon,
  TrashIcon,
  ArchiveIcon,
  RestoreIcon,
  StarIcon,
  PinIcon,
  ExternalLinkIcon,
} from "../../components/icons";
import { initialsFrom } from "../home/mappers";
import { useI18n } from "../../i18n/I18nProvider";
import { cn } from "../../lib/cn";
import styles from "./BookmarkCard.module.css";

interface BookmarkCardProps {
  bookmark: BookmarkDTO;
  onEdit: () => void;
  onDelete: () => void;
  onArchive: () => void;
  onRestore: () => void;
  onOpen: () => void;
  /** Multi-select mode */
  selectable?: boolean;
  selected?: boolean;
  onToggleSelect?: () => void;
}

/** A single bookmark row/card with status-aware actions and optional multi-select. */
export function BookmarkCard({
  bookmark,
  onEdit,
  onDelete,
  onArchive,
  onRestore,
  onOpen,
  selectable = false,
  selected = false,
  onToggleSelect,
}: BookmarkCardProps) {
  const { t } = useI18n();
  const isTrash = bookmark.status === "trash";
  const initials = initialsFrom(bookmark.title, bookmark.domain);

  return (
    <div className={cn(styles.card, isTrash && styles.trashed, selected && styles.selected)}>
      {selectable ? (
        <label className={styles.checkWrap}>
          <input
            type="checkbox"
            className={styles.checkbox}
            checked={selected}
            onChange={onToggleSelect}
            aria-label={t("card.selectBookmark", { title: bookmark.title || bookmark.domain })}
          />
          <span className={styles.checkMark} />
        </label>
      ) : null}

      <a
        className={styles.main}
        href={bookmark.url}
        target="_blank"
        rel="noreferrer noopener"
        onClick={onOpen}
      >
        <span className={styles.badge}>{initials}</span>
        <span className={styles.body}>
          <span className={styles.titleRow}>
            {bookmark.pinned ? (
              <PinIcon className={cn(styles.flag, styles.flagPinned)} />
            ) : null}
            {bookmark.favorite ? (
              <StarIcon className={cn(styles.flag, styles.flagFav)} />
            ) : null}
            <span className={styles.title}>{bookmark.title || bookmark.domain}</span>
          </span>
          <span className={styles.domain}>{bookmark.domain}</span>
          {bookmark.description ? (
            <span className={styles.desc}>{bookmark.description}</span>
          ) : null}
          {bookmark.tags && bookmark.tags.length > 0 ? (
            <span className={styles.tags}>
              {bookmark.tags.slice(0, 4).map((t) => (
                <span
                  key={t.id}
                  className={styles.tag}
                  style={t.color ? { ["--tag" as string]: t.color } : undefined}
                >
                  {t.name}
                </span>
              ))}
              {bookmark.tags.length > 4 ? (
                <span className={styles.tagMore}>+{bookmark.tags.length - 4}</span>
              ) : null}
            </span>
          ) : null}
        </span>
      </a>

      <div className={styles.actions}>
        <IconButton label={t("card.open")} onClick={onOpen}>
          <ExternalLinkIcon />
        </IconButton>
        {isTrash ? (
          <>
            <IconButton label={t("card.restore")} onClick={onRestore}>
              <RestoreIcon />
            </IconButton>
            <IconButton label={t("card.deletePermanently")} onClick={onDelete} variant="danger">
              <TrashIcon />
            </IconButton>
          </>
        ) : (
          <>
            <IconButton label={t("card.edit")} onClick={onEdit}>
              <EditIcon />
            </IconButton>
            <IconButton label={t("card.archive")} onClick={onArchive}>
              <ArchiveIcon />
            </IconButton>
            <IconButton label={t("card.moveToTrash")} onClick={onDelete} variant="danger">
              <TrashIcon />
            </IconButton>
          </>
        )}
      </div>
    </div>
  );
}
