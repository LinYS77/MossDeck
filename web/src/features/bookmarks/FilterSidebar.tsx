import type { CategoryDTO, TagDTO } from "../../lib/api/bookmarks";
import type { BookmarksFilters, BookmarkStatusFilter } from "./useBookmarksPage";
import { cn } from "../../lib/cn";
import { useI18n } from "../../i18n/I18nProvider";
import { BookmarkIcon, ArchiveIcon, TrashIcon, StarIcon, PinIcon } from "../../components/icons";
import styles from "./FilterSidebar.module.css";

interface FilterSidebarProps {
  filters: BookmarksFilters;
  categories: CategoryDTO[];
  tags: TagDTO[];
  onChange: (patch: Partial<BookmarksFilters>) => void;
}

const STATUS_SECTIONS: { value: BookmarkStatusFilter; key: string; icon: typeof BookmarkIcon }[] = [
  { value: "active", key: "filters.active", icon: BookmarkIcon },
  { value: "archived", key: "filters.archived", icon: ArchiveIcon },
  { value: "trash", key: "filters.trash", icon: TrashIcon },
];

/** Left rail: status tabs, category list, tag cloud, quick toggles. */
export function FilterSidebar({
  filters,
  categories,
  tags,
  onChange,
}: FilterSidebarProps) {
  const { t } = useI18n();

  return (
    <div className={styles.rail}>
      <section className={styles.section}>
        <h3 className={styles.heading}>{t("filters.status")}</h3>
        <ul className={styles.nav}>
          {STATUS_SECTIONS.map(({ value, key, icon: Icon }) => (
            <li key={value}>
              <button
                type="button"
                className={cn(styles.navItem, filters.status === value && styles.navItemActive)}
                onClick={() => onChange({ status: value })}
              >
                <Icon className={styles.navIcon} />
                <span>{t(key)}</span>
              </button>
            </li>
          ))}
        </ul>
      </section>

      <section className={styles.section}>
        <div className={styles.headingRow}>
          <h3 className={styles.heading}>{t("filters.categories")}</h3>
          <span className={styles.count}>{categories.length}</span>
        </div>
        <ul className={styles.nav}>
          {categories.map((c) => (
            <li key={c.id}>
              <button
                type="button"
                className={cn(
                  styles.navItem,
                  filters.categoryId === c.id && styles.navItemActive,
                  c.archived && styles.navItemMuted,
                )}
                onClick={() => onChange({ categoryId: c.id })}
              >
                <span className={styles.dot} aria-hidden />
                <span>{c.name}</span>
                <span className={styles.navMeta}>{c.bookmarkCount}</span>
              </button>
            </li>
          ))}
        </ul>
      </section>

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
                onClick={() =>
                  onChange({ tagId: filters.tagId === t.id ? null : t.id })
                }
              >
                {t.name}
              </button>
            ))}
          </div>
        )}
      </section>

      <section className={styles.section}>
        <h3 className={styles.heading}>{t("filters.quickFilters")}</h3>
        <div className={styles.toggles}>
          <button
            type="button"
            className={cn(styles.toggle, filters.favorite && styles.toggleOn)}
            onClick={() => onChange({ favorite: !filters.favorite })}
          >
            <StarIcon className={styles.toggleIcon} />
            {t("filters.favorites")}
          </button>
          <button
            type="button"
            className={cn(styles.toggle, filters.pinned && styles.toggleOn)}
            onClick={() => onChange({ pinned: !filters.pinned })}
          >
            <PinIcon className={styles.toggleIcon} />
            {t("filters.pinned")}
          </button>
        </div>
      </section>
    </div>
  );
}
