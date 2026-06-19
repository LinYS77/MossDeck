import { useCallback, useEffect, useRef, useState } from "react";
import * as api from "../../lib/api/readLater";
import type { ReadLaterDTO, ReadLaterListQuery } from "../../lib/api/readLater";
import {
  listTags,
  createTag,
  updateTag,
  deleteTag,
} from "../../lib/api/bookmarks";
import type {
  TagDTO,
  CreateTagParams,
  UpdateTagParams,
} from "../../lib/api/bookmarks";
import { ApiError } from "../../lib/api/client";

/** Sidebar state filter. "queue" is the default and maps to *no* `state`
 *  param, which the backend interprets as the active reading queue
 *  (unread + reading). The others pass through verbatim. */
export type ReadLaterStateFilter =
  | "queue"
  | "unread"
  | "reading"
  | "archived"
  | "trash";

/** Sort keys (snake_case, as the backend orderBy accepts). */
export type ReadLaterSort =
  | "created_desc"
  | "created_asc"
  | "updated_desc"
  | "updated_asc"
  | "priority_desc"
  | "priority_asc"
  | "title_asc";

export interface ReadLaterFilters {
  q: string;
  state: ReadLaterStateFilter;
  tagId: number | null;
  domain: string;
  favorite: boolean;
  priority: number | null;
  sort: ReadLaterSort;
}

export const DEFAULT_FILTERS: ReadLaterFilters = {
  q: "",
  state: "queue",
  tagId: null,
  domain: "",
  favorite: false,
  priority: null,
  sort: "created_desc",
};

export interface ReadLaterPageState {
  loading: boolean;
  items: ReadLaterDTO[];
  total: number;
  page: number;
  pageSize: number;
  tags: TagDTO[];
  error: string | null;
  /** Transient action error (e.g. a failed create or duplicate URL). */
  actionError: string | null;
  setActionError: (msg: string | null) => void;
  filters: ReadLaterFilters;
  setFilters: React.Dispatch<React.SetStateAction<ReadLaterFilters>>;
  setPage: (page: number) => void;
  reload: () => void;
  createItem: (p: api.CreateReadLaterParams) => Promise<void>;
  updateItem: (id: number, p: api.UpdateReadLaterParams) => Promise<void>;
  removeItem: (id: number) => Promise<void>;
  archiveItem: (id: number) => Promise<void>;
  restoreItem: (id: number) => Promise<void>;
  /** Permanently delete a trashed item. Fails if the item is not in trash. */
  purgeItem: (id: number) => Promise<void>;
  /** Best-effort open: records the open (unread -> reading) and reloads.
   *  Errors are swallowed so a failed recording never blocks navigation. */
  openItem: (id: number) => Promise<void>;
  createTag: (p: CreateTagParams) => Promise<TagDTO>;
  updateTag: (id: number, p: UpdateTagParams) => Promise<void>;
  removeTag: (id: number) => Promise<void>;
}

function friendly(err: unknown, fallback: string): string {
  if (err instanceof ApiError) return err.message || fallback;
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

/**
 * Orchestrates the read-later management page: loads the item list + tags,
 * owns the filter/pagination/sort state, and exposes typed mutations that
 * reload the affected data. Components stay presentational; this hook is the
 * single source of truth — mirroring {@link useBookmarksPage}.
 */
export function useReadLaterPage(): ReadLaterPageState {
  const [loading, setLoading] = useState(true);
  const [items, setItems] = useState<ReadLaterDTO[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize] = useState(24);
  const [tags, setTags] = useState<TagDTO[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [filters, setFilters] = useState<ReadLaterFilters>(DEFAULT_FILTERS);

  const ac = useRef<AbortController | null>(null);

  const reloadTags = useCallback(async () => {
    try {
      const tgs = await listTags();
      setTags(tgs);
    } catch (err) {
      // Tags are secondary; surface as a list-level error only when there's
      // nothing more important already showing.
      setError(friendly(err, "Couldn't load tags."));
    }
  }, []);

  const reloadItems = useCallback(async () => {
    ac.current?.abort();
    const controller = new AbortController();
    ac.current = controller;
    setLoading(true);
    setError(null);
    const query: ReadLaterListQuery = {
      q: filters.q.trim() || undefined,
      // "queue" → no state param (backend default = unread + reading).
      state: filters.state === "queue" ? undefined : filters.state,
      tagId: filters.tagId ?? undefined,
      domain: filters.domain.trim() || undefined,
      favorite: filters.favorite || undefined,
      priority: filters.priority ?? undefined,
      page,
      pageSize,
      sort: filters.sort,
    };
    try {
      const res = await api.listReadLater(query, controller.signal);
      if (controller.signal.aborted) return;
      setItems(res.items);
      setTotal(res.total);
    } catch (err) {
      if (controller.signal.aborted) return;
      setError(friendly(err, "Couldn't load your reading queue."));
      setItems([]);
      setTotal(0);
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [filters, page, pageSize]);

  // Load tags once on mount.
  useEffect(() => {
    void reloadTags();
  }, [reloadTags]);

  // Reload items whenever filters or page change.
  useEffect(() => {
    void reloadItems();
  }, [reloadItems]);

  const mutate = useCallback(
    async (
      fn: () => Promise<unknown>,
      fallback: string,
      opts: { reloadTags?: boolean } = {},
    ) => {
      setActionError(null);
      try {
        await fn();
        await Promise.all([
          reloadItems(),
          opts.reloadTags ? reloadTags() : Promise.resolve(),
        ]);
      } catch (err) {
        setActionError(friendly(err, fallback));
        throw err;
      }
    },
    [reloadItems, reloadTags],
  );

  return {
    loading,
    items,
    total,
    page,
    pageSize,
    tags,
    error,
    actionError,
    setActionError,
    filters,
    setFilters,
    setPage,
    reload: reloadItems,
    createItem: (p) => mutate(() => api.createReadLater(p), "Couldn't save the item."),
    updateItem: (id, p) => mutate(() => api.updateReadLater(id, p), "Couldn't save the item."),
    removeItem: (id) => mutate(() => api.deleteReadLater(id), "Couldn't move the item to trash."),
    purgeItem: (id) => mutate(() => api.purgeReadLater(id), "Couldn't permanently delete the item."),
    archiveItem: (id) => mutate(() => api.archiveReadLater(id), "Couldn't archive the item."),
    restoreItem: (id) => mutate(() => api.restoreReadLater(id), "Couldn't restore the item."),
    openItem: async (id) => {
      // Best-effort: never block the user from opening the link. A failed
      // recording is swallowed; on success we refresh so the state updates.
      try {
        await api.openReadLater(id);
        await reloadItems();
      } catch {
        /* intentionally ignored — navigation proceeds regardless */
      }
    },
    createTag: async (p) => {
      const tag = await createTag(p);
      await reloadTags();
      return tag;
    },
    updateTag: (id, p) =>
      mutate(() => updateTag(id, p), "Couldn't save the tag.", { reloadTags: true }),
    removeTag: (id) =>
      mutate(() => deleteTag(id), "Couldn't remove the tag.", { reloadTags: true }),
  };
}
