/** Bookmarks + categories + tags API. */

import { request, type QueryValue } from "./client";

// =====================================================================
// DTOs (match the backend handler JSON shapes, camelCase)
// =====================================================================

export interface CategoryDTO {
  id: number;
  name: string;
  slug?: string;
  type: string;
  icon?: string;
  color?: string;
  parentId?: number;
  sortOrder: number;
  archived: boolean;
  showOnHome: boolean;
  bookmarkCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface TagDTO {
  id: number;
  name: string;
  color?: string;
  bookmarkCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface BookmarkDTO {
  id: number;
  url: string;
  title: string;
  description?: string;
  domain: string;
  categoryId?: number;
  tags?: TagDTO[];
  pinned: boolean;
  favorite: boolean;
  sortOrder: number;
  clickCount: number;
  status: string;
  lastOpenedAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface BookmarkListDTO {
  items: BookmarkDTO[];
  page: number;
  pageSize: number;
  total: number;
}

export interface BookmarkListQuery {
  q?: string;
  status?: string;
  categoryId?: number;
  tagId?: number;
  tag?: string;
  domain?: string;
  favorite?: boolean;
  pinned?: boolean;
  page?: number;
  pageSize?: number;
  sort?: string;
  /** Index signature so the query bag satisfies the client's record type. */
  [key: string]: QueryValue;
}

export type DuplicateMode = "skip" | "update" | "duplicate";

export interface ImportSampleDTO {
  kind: string; // "skipped" | "failed"
  url?: string;
  reason: string;
}

export interface ImportResultDTO {
  total: number;
  processed: number;
  created: number;
  skipped: number;
  updated: number;
  failed: number;
  limitReached?: boolean;
  duplicateMode: DuplicateMode;
  samples?: ImportSampleDTO[];
}

// =====================================================================
// Request params
// =====================================================================

export interface CreateBookmarkParams {
  url: string;
  title: string;
  description?: string;
  categoryId?: number;
  tagIds?: number[];
  pinned?: boolean;
  favorite?: boolean;
  sortOrder?: number;
}

export type UpdateBookmarkParams = {
  url?: string;
  title?: string;
  description?: string;
  categoryId?: number | null;
  tagIds?: number[];
  pinned?: boolean;
  favorite?: boolean;
  sortOrder?: number;
  status?: string;
};

export interface CreateCategoryParams {
  name: string;
  icon?: string;
  color?: string;
  parentId?: number;
  sortOrder?: number;
}

export interface UpdateCategoryParams {
  name?: string;
  icon?: string | null;
  color?: string | null;
  sortOrder?: number;
  archived?: boolean;
  showOnHome?: boolean;
}

export interface CreateTagParams {
  name: string;
  color?: string;
}

export interface UpdateTagParams {
  name?: string;
  color?: string | null;
}

// =====================================================================
// Categories
// =====================================================================

export function listCategories(signal?: AbortSignal): Promise<CategoryDTO[]> {
  return request<CategoryDTO[]>("GET", "/api/v1/categories", { signal });
}

export function createCategory(params: CreateCategoryParams): Promise<CategoryDTO> {
  return request<CategoryDTO>("POST", "/api/v1/categories", { body: params });
}

export function updateCategory(id: number, params: UpdateCategoryParams): Promise<CategoryDTO> {
  return request<CategoryDTO>("PATCH", `/api/v1/categories/${id}`, { body: params });
}

export function deleteCategory(id: number): Promise<void> {
  return request<void>("DELETE", `/api/v1/categories/${id}`);
}

// =====================================================================
// Tags
// =====================================================================

export function listTags(signal?: AbortSignal): Promise<TagDTO[]> {
  return request<TagDTO[]>("GET", "/api/v1/tags", { signal });
}

export function createTag(params: CreateTagParams): Promise<TagDTO> {
  return request<TagDTO>("POST", "/api/v1/tags", { body: params });
}

export function updateTag(id: number, params: UpdateTagParams): Promise<TagDTO> {
  return request<TagDTO>("PATCH", `/api/v1/tags/${id}`, { body: params });
}

export function deleteTag(id: number): Promise<void> {
  return request<void>("DELETE", `/api/v1/tags/${id}`);
}

// =====================================================================
// Bookmarks
// =====================================================================

export function listBookmarks(
  query: BookmarkListQuery = {},
  signal?: AbortSignal,
): Promise<BookmarkListDTO> {
  return request<BookmarkListDTO>("GET", "/api/v1/bookmarks", { query, signal });
}

export function getBookmark(id: number): Promise<BookmarkDTO> {
  return request<BookmarkDTO>("GET", `/api/v1/bookmarks/${id}`);
}

export function createBookmark(params: CreateBookmarkParams): Promise<BookmarkDTO> {
  return request<BookmarkDTO>("POST", "/api/v1/bookmarks", { body: params });
}

export function updateBookmark(id: number, params: UpdateBookmarkParams): Promise<BookmarkDTO> {
  return request<BookmarkDTO>("PATCH", `/api/v1/bookmarks/${id}`, { body: params });
}

export function deleteBookmark(id: number): Promise<BookmarkDTO> {
  return request<BookmarkDTO>("DELETE", `/api/v1/bookmarks/${id}`);
}

export function restoreBookmark(id: number): Promise<BookmarkDTO> {
  return request<BookmarkDTO>("POST", `/api/v1/bookmarks/${id}/restore`);
}

export function archiveBookmark(id: number): Promise<BookmarkDTO> {
  return request<BookmarkDTO>("POST", `/api/v1/bookmarks/${id}/archive`);
}

export function openBookmark(id: number): Promise<void> {
  return request<void>("POST", `/api/v1/bookmarks/${id}/open`);
}

// =====================================================================
// Import (Netscape HTML)
// =====================================================================

export function importBookmarksHTML(
  file: File | string,
  mode: DuplicateMode = "skip",
): Promise<ImportResultDTO> {
  const form = new FormData();
  if (typeof file === "string") {
    // JSON { html } shape accepted by the backend; handy for tests/paste.
    return request<ImportResultDTO>("POST", "/api/v1/bookmarks/import/html", {
      body: { html: file },
      query: { duplicateMode: mode },
    });
  }
  form.append("file", file);
  return request<ImportResultDTO>("POST", "/api/v1/bookmarks/import/html", {
    form,
    query: { duplicateMode: mode },
  });
}
