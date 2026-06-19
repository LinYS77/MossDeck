import { useCallback, useEffect, useRef, useState } from "react";
import {
  listBookmarks,
  listCategories,
  openBookmark,
} from "../../lib/api/bookmarks";
import { listReadLater } from "../../lib/api/readLater";
import type { BookmarkDTO, CategoryDTO } from "../../lib/api/bookmarks";
import type { ReadLaterDTO } from "../../lib/api/readLater";
import { useI18n } from "../../i18n/I18nProvider";
import { useQuickAccessLimit } from "../../lib/deviceSettings";
import {
  deriveQuickLinks,
  mapBookmarksToGroups,
  mapReadLaterItems,
} from "./mappers";
import type {
  BookmarkGroup,
  QuickLink,
  ReadLaterItem,
} from "../../lib/types";

/** Homepage data status. */
export type HomeDataStatus = "loading" | "ready" | "error";

export interface HomeData {
  status: HomeDataStatus;
  /** Per-section error message (null when the section is fine). */
  errors: { bookmarks: string | null; readLater: string | null };
  categories: CategoryDTO[];
  bookmarks: BookmarkDTO[];
  groups: BookmarkGroup[];
  quickLinks: QuickLink[];
  readLater: ReadLaterItem[];
  /** Record a quick-link open so the click-count ordering stays fresh. */
  recordQuickLinkOpen: (id: string) => void;
  reload: (silent?: boolean) => void;
}

const EMPTY: HomeDataState = {
  categories: [],
  bookmarks: [],
  readLater: [],
};

interface HomeDataState {
  categories: CategoryDTO[];
  bookmarks: BookmarkDTO[];
  readLater: ReadLaterDTO[];
}

/**
 * Loads the homepage's three read paths and maps them to the view models the
 * existing components expect. Sections fail independently so a single broken
 * source (e.g. empty read-later) never blanks the whole page.
 */
export function useHomeData(): HomeData {
  const [status, setStatus] = useState<HomeDataStatus>("loading");
  const { t } = useI18n();
  const { limit: quickAccessLimit } = useQuickAccessLimit();
  const [data, setData] = useState<HomeDataState>(EMPTY);
  const [errors, setErrors] = useState<{
    bookmarks: string | null;
    readLater: string | null;
  }>({ bookmarks: null, readLater: null });

  const inFlight = useRef<AbortController | null>(null);

  const load = useCallback((silent = false) => {
    inFlight.current?.abort();
    const ac = new AbortController();
    inFlight.current = ac;

    if (!silent) setStatus("loading");
    setErrors({ bookmarks: null, readLater: null });

    // Categories are decorative grouping metadata; a failure just yields an
    // "Uncategorized" bucket, so swallow it.
    const categoriesPromise = listCategories(ac.signal)
      .catch(() => [] as CategoryDTO[]);

    // Bookmarks + read-later carry their own error surfaces.
    const bookmarksPromise = listBookmarks(
      { pageSize: 100, sort: "created_desc" },
      ac.signal,
    )
      .then((res) => res.items)
      .catch((err) => {
        if (ac.signal.aborted) throw err;
        return Promise.reject<never>(err);
      });

    const readLaterPromise = listReadLater({ pageSize: 8 }, ac.signal)
      .then((res) => res.items)
      .catch((err) => {
        if (ac.signal.aborted) throw err;
        return Promise.reject<never>(err);
      });

    void Promise.allSettled([categoriesPromise, bookmarksPromise, readLaterPromise]).then(
      ([catsRes, bmRes, rlRes]) => {
        if (ac.signal.aborted) return;

        const categories =
          catsRes.status === "fulfilled" ? catsRes.value : [];
        const bookmarks =
          bmRes.status === "fulfilled" ? bmRes.value : [];
        const readLater =
          rlRes.status === "fulfilled" ? rlRes.value : [];

        setData({ categories, bookmarks, readLater });
        setErrors({
          bookmarks:
            bmRes.status === "rejected"
              ? bmRes.reason instanceof Error
                ? bmRes.reason.message
                : "Couldn't load bookmarks"
              : null,
          readLater:
            rlRes.status === "rejected"
              ? rlRes.reason instanceof Error
                ? rlRes.reason.message
                : "Couldn't load read-later"
              : null,
        });
        setStatus("ready");
      },
    );
  }, []);

  useEffect(() => {
    load();
    return () => inFlight.current?.abort();
  }, [load]);

  // Reload when the page becomes visible again (e.g. navigating back from
  // bookmarks after an import that created new categories).
  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === "visible") load();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => document.removeEventListener("visibilitychange", onVisible);
  }, [load]);

  const groups = mapBookmarksToGroups(data.bookmarks, data.categories, t("common.uncategorized"));
  const quickLinks = deriveQuickLinks(data.bookmarks, quickAccessLimit);
  const readLaterView = mapReadLaterItems(data.readLater);

  // Fire-and-forget open recording for quick links. Failures (e.g. mock ids
  // or network blips) are swallowed so the external link always opens and the
  // click is simply not counted this time.
  const recordQuickLinkOpen = useCallback((id: string) => {
    const numId = Number(id);
    if (!Number.isFinite(numId) || numId <= 0) return;
    void openBookmark(numId).catch(() => {
      /* ignore — recording is best-effort */
    });
  }, []);

  return {
    status,
    errors,
    categories: data.categories,
    bookmarks: data.bookmarks,
    groups,
    quickLinks,
    readLater: readLaterView,
    recordQuickLinkOpen,
    reload: load,
  };
}
