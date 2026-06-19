/** Read-later API (read path used by the homepage). */

import { request, type QueryValue } from "./client";

export interface ReadLaterTagDTO {
  id: number;
  name: string;
  color?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ReadLaterDTO {
  id: number;
  url: string;
  title: string;
  excerpt?: string;
  author?: string;
  siteName?: string;
  faviconUrl?: string;
  coverImageUrl?: string;
  domain: string;
  readingTimeMinutes: number;
  state: string;
  priority: number;
  favorite: boolean;
  source?: string;
  lastOpenedAt?: string;
  archivedAt?: string;
  metadataStatus: string;
  tags?: ReadLaterTagDTO[];
  createdAt: string;
  updatedAt: string;
  deletedAt?: string;
}

export interface ReadLaterListDTO {
  items: ReadLaterDTO[];
  page: number;
  pageSize: number;
  total: number;
}

export interface ReadLaterListQuery {
  q?: string;
  state?: string;
  tagId?: number;
  tag?: string;
  domain?: string;
  favorite?: boolean;
  priority?: number;
  page?: number;
  pageSize?: number;
  sort?: string;
  /** Index signature so the query bag satisfies the client's record type. */
  [key: string]: QueryValue;
}

export function listReadLater(
  query: ReadLaterListQuery = {},
  signal?: AbortSignal,
): Promise<ReadLaterListDTO> {
  return request<ReadLaterListDTO>("GET", "/api/v1/read-later", { query, signal });
}

// =====================================================================
// Mutations (write path). The backend uses pointer fields with strict
// decoding (DisallowUnknownFields), so partial bodies are fine — undefined
// values are dropped by JSON.stringify. tagIds replaces the full set.
// =====================================================================

export interface CreateReadLaterParams {
  url: string;
  title?: string;
  excerpt?: string;
  author?: string;
  siteName?: string;
  readingTimeMinutes?: number;
  priority?: number;
  favorite?: boolean;
  source?: string;
  tagIds?: number[];
}

export type UpdateReadLaterParams = {
  url?: string;
  title?: string;
  excerpt?: string;
  author?: string;
  siteName?: string;
  readingTimeMinutes?: number;
  state?: string;
  priority?: number;
  favorite?: boolean;
  source?: string;
  tagIds?: number[];
};

export function getReadLater(id: number): Promise<ReadLaterDTO> {
  return request<ReadLaterDTO>("GET", `/api/v1/read-later/${id}`);
}

export function createReadLater(params: CreateReadLaterParams): Promise<ReadLaterDTO> {
  return request<ReadLaterDTO>("POST", "/api/v1/read-later", { body: params });
}

export function updateReadLater(id: number, params: UpdateReadLaterParams): Promise<ReadLaterDTO> {
  return request<ReadLaterDTO>("PATCH", `/api/v1/read-later/${id}`, { body: params });
}

/** Soft-delete (move to trash). Returns the trashed item. There is no
 *  hard-delete/purge endpoint on the backend. */
export function deleteReadLater(id: number): Promise<ReadLaterDTO> {
  return request<ReadLaterDTO>("DELETE", `/api/v1/read-later/${id}`);
}

/** Restore from trash/archived back to unread. */
export function restoreReadLater(id: number): Promise<ReadLaterDTO> {
  return request<ReadLaterDTO>("POST", `/api/v1/read-later/${id}/restore`);
}

/** Archive the item (hidden from the default queue). */
export function archiveReadLater(id: number): Promise<ReadLaterDTO> {
  return request<ReadLaterDTO>("POST", `/api/v1/read-later/${id}/archive`);
}

/** Permanently delete a trashed item. Only succeeds when state=trash;
 *  returns 400 for non-trash items and 404 for missing items. */
export function purgeReadLater(id: number): Promise<void> {
  return request<void>("DELETE", `/api/v1/read-later/${id}/purge`);
}

/** Record an open: sets lastOpenedAt and moves unread -> reading. */
export function openReadLater(id: number): Promise<ReadLaterDTO> {
  return request<ReadLaterDTO>("POST", `/api/v1/read-later/${id}/open`);
}
