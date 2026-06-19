import type { BookmarkGroup as BookmarkGroupType } from "../lib/types";
import { CodeIcon, SparklesIcon, BookIcon, ExternalLinkIcon } from "./icons";
import { GlassPanel } from "./GlassPanel";
import { initialsFrom } from "../features/home/mappers";
import { useI18n } from "../i18n/I18nProvider";
import styles from "./BookmarkGroup.module.css";

const ICONS: Record<string, React.FC<React.SVGProps<SVGSVGElement>>> = {
  code: CodeIcon,
  sparkles: SparklesIcon,
  book: BookIcon,
};

/** A glass card listing the bookmarks filed under one group. */
export function BookmarkGroup({ group }: { group: BookmarkGroupType }) {
  const { t } = useI18n();
  const Icon = (group.icon && ICONS[group.icon]) || BookmarkGlyph;
  const count = group.bookmarks.length;

  return (
    <GlassPanel as="section" className={styles.group}>
      <header className={styles.header}>
        <span className={styles.iconWrap}>
          <Icon className={styles.icon} />
        </span>
        <div className={styles.titles}>
          <h3 className={styles.name}>{group.name}</h3>
          <span className={styles.count}>
            {count === 1
              ? t("home.bookmarkCount", { count })
              : t("home.bookmarkCount_plural", { count })}
          </span>
        </div>
      </header>

      <ul className={styles.list}>
        {group.bookmarks.map((b) => (
          <li key={b.id}>
            <a
              className={styles.item}
              href={b.url}
              target="_blank"
              rel="noreferrer noopener"
            >
              <span className={styles.favicon}>
                {initialsFrom(b.title, b.domain)}
              </span>
              <span className={styles.itemBody}>
                <span className={styles.itemTitle}>{b.title}</span>
                <span className={styles.itemDomain}>{b.domain}</span>
              </span>
              <ExternalLinkIcon className={styles.ext} />
            </a>
          </li>
        ))}
      </ul>
    </GlassPanel>
  );
}

function BookmarkGlyph(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg
      {...props}
      width={24}
      height={24}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.7}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <path d="M6 4h12v16l-6-4-6 4z" />
    </svg>
  );
}

export function BookmarkGroupsGrid({ groups }: { groups: BookmarkGroupType[] }) {
  // Show every group the home data layer produced. Which categories appear is
  // decided by the showOnHome flag (mapBookmarksToGroups), not a hard cap here
  // — the old slice(0, 2) silently hid most of the user's categories.
  return (
    <div className={styles.grid}>
      {groups.map((g) => (
        <BookmarkGroup key={g.id} group={g} />
      ))}
    </div>
  );
}
