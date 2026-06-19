import { useEffect, useId, useRef, useState } from "react";
import type { TagDTO } from "../../lib/api/bookmarks";
import type { ReadLaterFilters, ReadLaterSort, ReadLaterStateFilter } from "./useReadLaterPage";
import { cn } from "../../lib/cn";
import { useI18n } from "../../i18n/I18nProvider";
import {
  InboxIcon,
  BookmarkIcon,
  BookIcon,
  ArchiveIcon,
  TrashIcon,
  StarIcon,
  CloseIcon,
  FlagIcon,
  ChevronDownIcon,
} from "../../components/icons";
import styles from "./ReadLaterFilters.module.css";

interface ReadLaterFiltersProps {
  filters: ReadLaterFilters;
  tags: TagDTO[];
  total: number;
  /** Distinct priorities present in the current view (sorted high → low). */
  priorities: number[];
  onChange: (patch: Partial<ReadLaterFilters>) => void;
}

const STATUS_SECTIONS: {
  value: ReadLaterStateFilter;
  key: string;
  icon: typeof InboxIcon;
}[] = [
  { value: "queue", key: "readlater.readingQueue", icon: InboxIcon },
  { value: "unread", key: "readlater.unread", icon: BookmarkIcon },
  { value: "reading", key: "readlater.reading", icon: BookIcon },
  { value: "archived", key: "readlater.archived", icon: ArchiveIcon },
  { value: "trash", key: "readlater.trash", icon: TrashIcon },
];

export const SORT_OPTIONS: { value: ReadLaterSort; key: string }[] = [
  { value: "created_desc", key: "filters.sortNewest" },
  { value: "created_asc", key: "filters.sortOldest" },
  { value: "updated_desc", key: "filters.sortUpdated" },
  { value: "priority_desc", key: "filters.sortPriorityHigh" },
  { value: "priority_asc", key: "filters.sortPriorityLow" },
  { value: "title_asc", key: "filters.sortTitleAZ" },
];

/** Left rail: status tabs, active-domain chip, tag cloud, priority chips,
 *  favorite toggle, and a sort selector. */
export function ReadLaterFilters({
  filters,
  tags,
  total,
  priorities,
  onChange,
}: ReadLaterFiltersProps) {
  const { t } = useI18n();
  const [sortOpen, setSortOpen] = useState(false);
  const sortWrapRef = useRef<HTMLDivElement>(null);
  const sortMenuId = useId();
  const selectedSort = SORT_OPTIONS.find((option) => option.value === filters.sort) ?? SORT_OPTIONS[0];

  useEffect(() => {
    if (!sortOpen) return;
    const onPointerDown = (event: MouseEvent) => {
      if (!sortWrapRef.current?.contains(event.target as Node)) setSortOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setSortOpen(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [sortOpen]);

  return (
    <div className={styles.rail}>
      <section className={styles.section}>
        <h3 className={styles.heading}>{t("filters.status")}</h3>
        <ul className={styles.nav}>
          {STATUS_SECTIONS.map(({ value, key, icon: Icon }) => (
            <li key={value}>
              <button
                type="button"
                className={cn(styles.navItem, filters.state === value && styles.navItemActive)}
                onClick={() => onChange({ state: value })}
              >
                <Icon className={styles.navIcon} />
                <span>{t(key)}</span>
                {value === "queue" ? <span className={styles.navMeta}>{total}</span> : null}
              </button>
            </li>
          ))}
        </ul>
      </section>

      {filters.domain ? (
        <section className={styles.section}>
          <h3 className={styles.heading}>{t("filters.domain")}</h3>
          <button
            type="button"
            className={cn(styles.navItem, styles.navItemActive)}
            onClick={() => onChange({ domain: "" })}
          >
            <span className={styles.navMetaIcon}>
              <CloseIcon className={styles.clearIcon} />
            </span>
            <span className={styles.domainLabel}>{filters.domain}</span>
            <span className={styles.navMeta}>{t("filters.clear")}</span>
          </button>
        </section>
      ) : null}

      <section className={styles.section}>
        <div className={styles.headingRow}>
          <h3 className={styles.heading}>{t("filters.tags")}</h3>
          <span className={styles.count}>{tags.length}</span>
        </div>
        {tags.length === 0 ? (
          <p className={styles.empty}>{t("filters.noTags")}</p>
        ) : (
          <div className={styles.tagCloud}>
            {tags.map((t) => (
              <button
                key={t.id}
                type="button"
                className={cn(styles.tag, filters.tagId === t.id && styles.tagActive)}
                style={t.color ? { ["--tag" as string]: t.color } : undefined}
                onClick={() => onChange({ tagId: filters.tagId === t.id ? null : t.id })}
              >
                {t.name}
              </button>
            ))}
          </div>
        )}
      </section>

      {priorities.length > 0 ? (
        <section className={styles.section}>
          <h3 className={styles.heading}>{t("filters.priority")}</h3>
          <div className={styles.priorityCloud}>
            <button
              type="button"
              className={cn(styles.tag, filters.priority === null && styles.tagActive)}
              onClick={() => onChange({ priority: null })}
            >
              {t("filters.all")}
            </button>
            {priorities.map((p) => (
              <button
                key={p}
                type="button"
                className={cn(styles.tag, filters.priority === p && styles.tagActive)}
                onClick={() => onChange({ priority: filters.priority === p ? null : p })}
              >
                <FlagIcon className={styles.flagIcon} />
                {p}
              </button>
            ))}
          </div>
        </section>
      ) : null}

      <section className={styles.section}>
        <h3 className={styles.heading}>{t("filters.quickFilters")}</h3>
        <button
          type="button"
          className={cn(styles.toggle, filters.favorite && styles.toggleOn)}
          onClick={() => onChange({ favorite: !filters.favorite })}
        >
          <StarIcon className={styles.toggleIcon} />
          {t("filters.favorites")}
        </button>
      </section>

      <section className={styles.section}>
        <h3 className={styles.heading}>{t("filters.sort")}</h3>
        <div className={styles.sortWrap} ref={sortWrapRef}>
          <button
            type="button"
            className={styles.sortButton}
            onClick={() => setSortOpen((open) => !open)}
            aria-haspopup="menu"
            aria-expanded={sortOpen}
            aria-controls={sortMenuId}
          >
            <span>{t(selectedSort.key)}</span>
            <ChevronDownIcon className={cn(styles.sortChevron, sortOpen && styles.sortChevronOpen)} />
          </button>
          {sortOpen ? (
            <div className={styles.sortMenu} id={sortMenuId} role="menu">
              {SORT_OPTIONS.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  role="menuitemradio"
                  aria-checked={option.value === filters.sort}
                  className={cn(styles.sortItem, option.value === filters.sort && styles.sortItemActive)}
                  onClick={() => {
                    onChange({ sort: option.value });
                  }}
                >
                  {t(option.key)}
                </button>
              ))}
            </div>
          ) : null}
        </div>
      </section>
    </div>
  );
}
