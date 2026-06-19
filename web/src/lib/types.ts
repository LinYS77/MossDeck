/** Shared domain types for the home view.
 *
 * These mirror (a subset of) the Go backend response shapes so the frontend
 * can switch from mock data to real API calls without reshaping. Field names
 * use camelCase as the API envelope does.
 */

export interface QuickLink {
  id: string;
  label: string;
  url: string;
  /** Short label used for the favicon tile, or an icon key. */
  icon: string;
  /** Brand colour used for the tile gradient. */
  color: string;
}

export interface BookmarkItem {
  id: string;
  title: string;
  url: string;
  domain: string;
  description?: string;
  icon?: string;
  color?: string;
}

export interface BookmarkGroup {
  id: string;
  name: string;
  icon?: string;
  bookmarks: BookmarkItem[];
}

export interface ReadLaterItem {
  id: string;
  title: string;
  url: string;
  domain: string;
  /** estimated reading time in minutes */
  readingTime: number;
  source?: string;
  favorite?: boolean;
}

export interface Wallpaper {
  slug: string;
  label: string;
  /** Built-in public path or uploaded data URL. */
  src: string;
  thumb: string;
  custom?: boolean;
}

export type SearchEngineId = "google" | "duckduckgo";
