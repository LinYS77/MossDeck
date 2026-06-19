import { useCallback, useEffect, useRef, useState } from "react";
import * as api from "../../lib/api/bookmarks";
import type {
  BookmarkDTO,
  BookmarkListQuery,
  CategoryDTO,
  TagDTO,
} from "../../lib/api/bookmarks";
import { ApiError } from "../../lib/api/client";

export type BookmarkStatusFilter = "active" | "archived" | "trash";

export interface BookmarksFilters {
  q: string;
  status: BookmarkStatusFilter;
  categoryId: number | null;
  tagId: number | null;
  favorite: boolean;
  pinned: boolean;
}

export const DEFAULT_FILTERS: BookmarksFilters = {
  q: "",
  status: "active",
  categoryId: null,
  tagId: null,
  favorite: false,
  pinned: false,
};

export interface BookmarksPageState {
  loading: boolean;
  bookmarks: BookmarkDTO[];
  total: number;
  page: number;
  pageSize: number;
  categories: CategoryDTO[];
  tags: TagDTO[];
  error: string | null;
  /** Transient action error (e.g. a failed create). */
  actionError: string | null;
  setActionError: (msg: string | null) => void;
  filters: BookmarksFilters;
  setFilters: React.Dispatch<React.SetStateAction<BookmarksFilters>>;
  setPage: (page: number) => void;
  reload: () => void;
  /** Reload bookmarks AND categories/tags (call after import). Uses
   *  independent AbortControllers so the two fetches don't cancel each other. */
  reloadAll: () => Promise<void>;
  createBookmark: (p: api.CreateBookmarkParams) => Promise<void>;
  updateBookmark: (id: number, p: api.UpdateBookmarkParams) => Promise<void>;
  removeBookmark: (id: number) => Promise<void>;
  archiveBookmark: (id: number) => Promise<void>;
  restoreBookmark: (id: number) => Promise<void>;
  openBookmark: (id: number) => Promise<void>;
  createCategory: (p: api.CreateCategoryParams) => Promise<CategoryDTO>;
  updateCategory: (id: number, p: api.UpdateCategoryParams) => Promise<void>;
  removeCategory: (id: number) => Promise<void>;
  createTag: (p: api.CreateTagParams) => Promise<TagDTO>;
  updateTag: (id: number, p: api.UpdateTagParams) => Promise<void>;
  removeTag: (id: number) => Promise<void>;
  /** Batch operations — process IDs sequentially, reload once at the end. */
  batchArchive: (ids: number[]) => Promise<void>;
  batchRemove: (ids: number[]) => Promise<void>;
  batchRestore: (ids: number[]) => Promise<void>;
}

function friendly(err: unknown, fallback: string): string {
  if (err instanceof ApiError) return err.message || fallback;
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

/**
 * Orchestrates the bookmarks management page: loads the bookmark list +
 * categories + tags, owns the filter/pagination state, and exposes typed
 * mutations that optimistically reload the affected data. Components stay
 * presentational; this hook is the single source of truth.
 */
export function useBookmarksPage(): BookmarksPageState {
  const [loading, setLoading] = useState(true);
  const [bookmarks, setBookmarks] = useState<BookmarkDTO[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(24);
  const [categories, setCategories] = useState<CategoryDTO[]>([]);
  const [tags, setTags] = useState<TagDTO[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [filters, setFilters] = useState<BookmarksFilters>(DEFAULT_FILTERS);

  const bmAC = useRef<AbortController | null>(null);
  const listAC = useRef<AbortController | null>(null);

  const reloadLists = useCallback(async () => {
    listAC.current?.abort();
    const controller = new AbortController();
    listAC.current = controller;
    setError(null);
    try {
      const [cats, tgs] = await Promise.all([
        api.listCategories(controller.signal),
        api.listTags(controller.signal),
      ]);
      if (controller.signal.aborted) return;
      setCategories(cats);
      setTags(tgs);
    } catch (err) {
      if (controller.signal.aborted) return;
      setError(friendly(err, "Couldn't load categories/tags."));
    }
  }, []);

  const reloadBookmarks = useCallback(async () => {
    bmAC.current?.abort();
    const controller = new AbortController();
    bmAC.current = controller;
    setLoading(true);
    setError(null);
    const query: BookmarkListQuery = {
      q: filters.q.trim() || undefined,
      status: filters.status,
      categoryId: filters.categoryId ?? undefined,
      tagId: filters.tagId ?? undefined,
      favorite: filters.favorite || undefined,
      pinned: filters.pinned || undefined,
      page,
      pageSize,
    };
    try {
      const res = await api.listBookmarks(query, controller.signal);
      if (controller.signal.aborted) return;
      setBookmarks(res.items);
      setTotal(res.total);
    } catch (err) {
      if (controller.signal.aborted) return;
      setError(friendly(err, "Couldn't load bookmarks."));
      setBookmarks([]);
      setTotal(0);
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [filters, page, pageSize]);

  // Load lists once on mount.
  useEffect(() => {
    void reloadLists();
  }, [reloadLists]);

  // Reload bookmarks when filters or page change.
  useEffect(() => {
    void reloadBookmarks();
  }, [reloadBookmarks]);

  const mutate = useCallback(
    async (fn: () => Promise<unknown>, fallback: string, reload = true) => {
      setActionError(null);
      try {
        await fn();
        if (reload) {
          await Promise.all([reloadBookmarks(), reloadLists()]);
        }
      } catch (err) {
        setActionError(friendly(err, fallback));
        throw err;
      }
    },
    [reloadBookmarks, reloadLists],
  );

  return {
    loading,
    bookmarks,
    total,
    page,
    pageSize,
    categories,
    tags,
    error,
    actionError,
    setActionError,
    filters,
    setFilters,
    setPage,
    reload: reloadBookmarks,
    /** Reload bookmarks AND categories/tags (call after import). Uses
     *  independent AbortControllers so the two fetches don't cancel each other. */
    reloadAll: () => Promise.all([reloadBookmarks(), reloadLists()]).then(() => undefined),
    createBookmark: (p) => mutate(() => api.createBookmark(p), "Couldn't create the bookmark."),
    updateBookmark: (id, p) =>
      mutate(() => api.updateBookmark(id, p), "Couldn't save the bookmark."),
    removeBookmark: (id) =>
      mutate(() => api.deleteBookmark(id), "Couldn't delete the bookmark."),
    archiveBookmark: (id) =>
      mutate(() => api.archiveBookmark(id), "Couldn't archive the bookmark."),
    restoreBookmark: (id) =>
      mutate(() => api.restoreBookmark(id), "Couldn't restore the bookmark."),
    openBookmark: (id) =>
      mutate(() => api.openBookmark(id), "Couldn't record the open.", false),
    createCategory: async (p) => {
      const cat = await api.createCategory(p);
      await reloadLists();
      return cat;
    },
    updateCategory: (id, p) =>
      mutate(() => api.updateCategory(id, p), "Couldn't save the category."),
    removeCategory: (id) =>
      mutate(() => api.deleteCategory(id), "Couldn't remove the category."),
    createTag: async (p) => {
      const tag = await api.createTag(p);
      await reloadLists();
      return tag;
    },
    updateTag: (id, p) =>
      mutate(() => api.updateTag(id, p), "Couldn't save the tag."),
    removeTag: (id) => mutate(() => api.deleteTag(id), "Couldn't remove the tag."),
    /** Batch operations — process IDs sequentially, reload once at the end. */
    batchArchive: (ids: number[]) =>
      mutate(
        async () => {
          for (const id of ids) await api.archiveBookmark(id);
        },
        "Couldn't archive some bookmarks.",
      ),
    batchRemove: (ids: number[]) =>
      mutate(
        async () => {
          for (const id of ids) await api.deleteBookmark(id);
        },
        "Couldn't delete some bookmarks.",
      ),
    batchRestore: (ids: number[]) =>
      mutate(
        async () => {
          for (const id of ids) await api.restoreBookmark(id);
        },
        "Couldn't restore some bookmarks.",
      ),
  };
}
