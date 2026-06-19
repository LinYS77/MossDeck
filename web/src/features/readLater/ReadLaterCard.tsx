import { useState } from "react";
import type { ReadLaterDTO } from "../../lib/api/readLater";
import { IconButton } from "../../components/ui";
import { useI18n } from "../../i18n/I18nProvider";
import {
  EditIcon,
  TrashIcon,
  ArchiveIcon,
  RestoreIcon,
  StarIcon,
  ExternalLinkIcon,
  ClockIcon,
  FlagIcon,
} from "../../components/icons";
import { colorFor, initialsFrom } from "../home/mappers";
import { cn } from "../../lib/cn";
import styles from "./ReadLaterCard.module.css";

interface ReadLaterCardProps {
  item: ReadLaterDTO;
  onOpen: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onArchive: () => void;
  onRestore: () => void;
  onPurge: () => void;
  onToggleFavorite: () => void;
  onFilterDomain: (domain: string) => void;
}

/** A single read-later card. The title is a real link (opens in a new tab and
 *  records the open best-effort); domain filters on click; the favorite star,
 *  state transitions and trash live in the action rail. */
export function ReadLaterCard({
  item,
  onOpen,
  onEdit,
  onDelete,
  onArchive,
  onRestore,
  onPurge,
  onToggleFavorite,
  onFilterDomain,
}: ReadLaterCardProps) {
  const isTrash = item.state === "trash";
  const isArchived = item.state === "archived";
  const [imgFailed, setImgFailed] = useState(false);
  const { t } = useI18n();

  const stateLabel = (s: string): string => {
    switch (s) {
      case "unread": return t("readlater.unread");
      case "reading": return t("readlater.reading");
      case "archived": return t("readlater.archived");
      case "trash": return t("readlater.trash");
      default: return s;
    }
  };

  const color = colorFor(item.domain || item.url);
  const initials = initialsFrom(item.title, item.domain);
  const showFavicon = !!item.faviconUrl && !imgFailed;

  return (
    <div className={cn(styles.card, styles[`state_${item.state}`] ?? styles.state_default)}>
      <a
        className={styles.main}
        href={item.url}
        target="_blank"
        rel="noreferrer noopener"
        onClick={() => void onOpen()}
      >
        <span className={styles.badge} style={{ background: `${color}22`, color }}>
          {showFavicon ? (
            <img
              className={styles.favicon}
              src={item.faviconUrl}
              alt=""
              onError={() => setImgFailed(true)}
            />
          ) : (
            initials
          )}
        </span>

        <span className={styles.body}>
          <span className={styles.titleRow}>
            {item.priority !== 0 ? (
              <span className={styles.priority} title={`Priority ${item.priority}`}>
                <FlagIcon className={styles.priorityIcon} />
                {item.priority}
              </span>
            ) : null}
            <span className={styles.statePill} data-state={item.state}>
              {stateLabel(item.state)}
            </span>
            <span className={styles.title}>{item.title || item.domain}</span>
          </span>

          <span className={styles.meta}>
            <button
              type="button"
              className={styles.domain}
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                onFilterDomain(item.domain);
              }}
              title={`Filter by ${item.domain}`}
            >
              {item.siteName || item.domain}
            </button>
            {item.readingTimeMinutes > 0 ? (
              <>
                <span className={styles.dot} aria-hidden>·</span>
                <span className={styles.reading}>
                  <ClockIcon className={styles.clock} />
                  {item.readingTimeMinutes} min
                </span>
              </>
            ) : null}
            {item.lastOpenedAt ? (
              <>
                <span className={styles.dot} aria-hidden>·</span>
                <span className={styles.opened} title={`Last opened ${item.lastOpenedAt}`}>
                  opened
                </span>
              </>
            ) : null}
          </span>

          {item.excerpt ? <span className={styles.excerpt}>{item.excerpt}</span> : null}

          {item.tags && item.tags.length > 0 ? (
            <span className={styles.tags}>
              {item.tags.slice(0, 4).map((t) => (
                <span
                  key={t.id}
                  className={styles.tag}
                  style={t.color ? { ["--tag" as string]: t.color } : undefined}
                >
                  {t.name}
                </span>
              ))}
              {item.tags.length > 4 ? (
                <span className={styles.tagMore}>+{item.tags.length - 4}</span>
              ) : null}
            </span>
          ) : null}
        </span>
      </a>

      <div className={styles.actions}>
        <IconButton
          label={item.favorite ? t("card.removeFromFavorites") : t("card.addToFavorites")}
          active={item.favorite}
          onClick={onToggleFavorite}
          className={cn(styles.favBtn, item.favorite && styles.favBtnOn)}
        >
          <StarIcon />
        </IconButton>
        <IconButton label={t("card.open")} onClick={onOpen}>
          <ExternalLinkIcon />
        </IconButton>
        <IconButton label={t("card.edit")} onClick={onEdit}>
          <EditIcon />
        </IconButton>
        {isTrash ? (
          <>
            <IconButton label={t("card.restore")} onClick={onRestore}>
              <RestoreIcon />
            </IconButton>
            <IconButton
              label={t("card.deletePermanently")}
              onClick={() => {
                if (window.confirm(t("card.confirmPurge"))) {
                  onPurge();
                }
              }}
              variant="danger"
            >
              <TrashIcon />
            </IconButton>
          </>
        ) : isArchived ? (
          <>
            <IconButton label={t("card.restore")} onClick={onRestore}>
              <RestoreIcon />
            </IconButton>
            <IconButton label={t("card.moveToTrash")} onClick={onDelete} variant="danger">
              <TrashIcon />
            </IconButton>
          </>
        ) : (
          <>
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
