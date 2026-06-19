/** Backup API – export and import user data. */

import { request } from "./client";

// ----- Export -----

export interface BackupExport {
  version: number;
  app: string;
  exportedAt: string;
  data: {
    categories: BackupCategory[];
    tags: BackupTag[];
    bookmarks: BackupBookmark[];
    readLaterItems: BackupReadLaterItem[];
  };
}

export interface BackupCategory {
  name: string;
  slug?: string;
  type: string;
  icon?: string;
  color?: string;
  sortOrder: number;
  archived: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export interface BackupTag {
  name: string;
  color?: string;
}

export interface BackupBookmark {
  url: string;
  title: string;
  description?: string;
  category?: string;
  tags?: string[];
  pinned: boolean;
  favorite: boolean;
  sortOrder: number;
  clickCount: number;
  status: string;
  createdAt?: string;
  updatedAt?: string;
  lastOpenedAt?: string;
}

export interface BackupReadLaterItem {
  url: string;
  title: string;
  excerpt?: string;
  author?: string;
  siteName?: string;
  tags?: string[];
  state: string;
  priority: number;
  favorite: boolean;
  readingTimeMinutes: number;
  source?: string;
  createdAt?: string;
  updatedAt?: string;
  lastOpenedAt?: string;
  archivedAt?: string;
}

/** Download a JSON backup of all user data. */
export function exportBackup(signal?: AbortSignal): Promise<BackupExport> {
  return request<BackupExport>("GET", "/api/v1/backup/export", { signal });
}

// ----- Import -----

export interface ImportRequest {
  mode: "merge" | "replace";
  backup: BackupExport;
}

export interface ImportSummary {
  mode: string;
  categories: CountSummary;
  tags: CountSummary;
  bookmarks: CountSummary;
  readLaterItems: CountSummary;
  errors?: string[];
  warnings?: string[];
}

export interface CountSummary {
  created: number;
  updated: number;
  skipped: number;
}

/** Import a previously exported backup. */
export function importBackup(req: ImportRequest): Promise<ImportSummary> {
  return request<ImportSummary>("POST", "/api/v1/backup/import", { body: req });
}
