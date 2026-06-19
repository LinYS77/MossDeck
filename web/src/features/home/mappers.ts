import type {
  BookmarkGroup,
  BookmarkItem,
  QuickLink,
  ReadLaterItem,
} from "../../lib/types";
import type {
  BookmarkDTO,
  CategoryDTO,
} from "../../lib/api/bookmarks";
import type { ReadLaterDTO } from "../../lib/api/readLater";

const UNCAT_ID = "uncategorized";

/** A calm, saturated palette for tiles/badges derived from a domain. */
const PALETTE = [
  "#6ba5b4",
  "#5a8a80",
  "#a3c4bc",
  "#7eb8c9",
  "#94b0a8",
  "#b8a4c9",
  "#8ba888",
  "#c4958a",
  "#9ab8d4",
  "#a8b894",
];

/** Deterministic colour for a string (domain/title), spread across the palette. */
export function colorFor(seed: string): string {
  let hash = 0;
  for (let i = 0; i < seed.length; i++) {
    hash = (hash << 5) - hash + seed.charCodeAt(i);
    hash |= 0;
  }
  return PALETTE[Math.abs(hash) % PALETTE.length];
}

/** 1–2 uppercase letters from a title/domain, for tiles without a real icon. */
export function initialsFrom(title: string, domain: string): string {
  const source = (title || domain || "").trim();
  if (!source) return "•";
  // Strip protocol/www and take the bare domain label for cleaner initials.
  const cleanDomain = domain.replace(/^https?:\/\//, "").replace(/^www\./, "");
  const words = source.replace(/[^\p{L}\p{N}\s.-]/gu, " ").split(/\s+/).filter(Boolean);
  if (words.length >= 2) {
    return (words[0][0] + words[1][0]).toUpperCase();
  }
  if (cleanDomain) {
    const label = cleanDomain.split(".")[0];
    if (label.length >= 2) return label.slice(0, 2).toUpperCase();
  }
  return source.slice(0, 2).toUpperCase();
}

function mapBookmark(b: BookmarkDTO): BookmarkItem {
  return {
    id: String(b.id),
    title: b.title || b.domain || b.url,
    url: b.url,
    domain: b.domain,
    description: b.description,
    color: colorFor(b.domain || b.url),
    icon: initialsFrom(b.title, b.domain),
  };
}

/** Group bookmarks by category, preserving category order and placing
 *  uncategorised items last under "Uncategorized". Only categories flagged
 *  showOnHome surface on the homepage (the user picks which categories appear
 *  via the category manager); empty categories are dropped to keep the home
 *  view tidy. The "Uncategorized" bucket has no flag and always shows when it
 *  has items, so links without a category are never silently hidden. */
export function mapBookmarksToGroups(
  bookmarks: BookmarkDTO[],
  categories: CategoryDTO[],
  uncategorizedLabel = "Uncategorized",
): BookmarkGroup[] {
  const order = categories
    .filter((c) => !c.archived && c.showOnHome)
    .map((c) => ({ id: String(c.id), name: c.name, icon: c.icon }));
  const byId = new Map<string, BookmarkItem[]>();
  for (const o of order) byId.set(o.id, []);
  byId.set(UNCAT_ID, []);

  for (const b of bookmarks) {
    const key = b.categoryId ? String(b.categoryId) : UNCAT_ID;
    const bucket = byId.get(key) ?? byId.get(UNCAT_ID)!;
    bucket.push(mapBookmark(b));
  }

  const groups: BookmarkGroup[] = [];
  for (const o of order) {
    const items = byId.get(o.id) ?? [];
    if (items.length > 0) groups.push({ ...o, bookmarks: items });
  }
  const uncategorized = byId.get(UNCAT_ID) ?? [];
  if (uncategorized.length > 0) {
    groups.push({ id: UNCAT_ID, name: uncategorizedLabel, bookmarks: uncategorized });
  }
  return groups;
}

/** Derive quick-link tiles: prefer pinned, then favourites, then most-clicked. */
export function deriveQuickLinks(bookmarks: BookmarkDTO[], max = 8): QuickLink[] {
  const sorted = [...bookmarks].sort((a, b) => {
    if (a.pinned !== b.pinned) return a.pinned ? -1 : 1;
    if (a.favorite !== b.favorite) return a.favorite ? -1 : 1;
    return b.clickCount - a.clickCount;
  });
  return sorted.slice(0, max).map((b) => ({
    id: String(b.id),
    label: b.title || b.domain || b.url,
    url: b.url,
    icon: initialsFrom(b.title, b.domain),
    color: colorFor(b.domain || b.url),
  }));
}

export function mapReadLaterItems(items: ReadLaterDTO[]): ReadLaterItem[] {
  return items.map((it) => ({
    id: String(it.id),
    title: it.title || it.domain || it.url,
    url: it.url,
    domain: it.domain,
    readingTime: it.readingTimeMinutes || 0,
    source: it.source || it.siteName,
    favorite: it.favorite,
  }));
}
