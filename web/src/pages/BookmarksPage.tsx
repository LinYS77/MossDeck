import { useEffect, useMemo, useState, useCallback } from "react";
import { Button, IconButton } from "../components/ui";
import { Modal } from "../components/Modal";
import { EmptyState } from "../components/EmptyState";
import { GlassSkeleton } from "../components/GlassSkeleton";
import {
  PlusIcon,
  UploadIcon,
  LayersIcon,
  BookmarkIcon,
  FolderIcon,
  TagIcon,
  FilterIcon,
  SearchIcon,
  ArchiveIcon,
  TrashIcon,
  RestoreIcon,
} from "../components/icons";
import { useBookmarksPage } from "../features/bookmarks/useBookmarksPage";
import type { BookmarksFilters } from "../features/bookmarks/useBookmarksPage";
import { FilterSidebar } from "../features/bookmarks/FilterSidebar";
import { BookmarkCard } from "../features/bookmarks/BookmarkCard";
import {
  BookmarkForm,
  bookmarkToForm,
  EMPTY_BOOKMARK_FORM,
  type BookmarkFormValue,
} from "../features/bookmarks/BookmarkForm";
import { CategoryManager } from "../features/bookmarks/CategoryManager";
import { TagManager } from "../features/bookmarks/TagManager";
import { ImportDialog } from "../features/bookmarks/ImportDialog";
import type { BookmarkDTO } from "../lib/api/bookmarks";
import { cn } from "../lib/cn";
import { useI18n } from "../i18n/I18nProvider";
import styles from "./BookmarksPage.module.css";

type ManageTab = "categories" | "tags";
type DialogKind = "create" | "edit" | "manage" | "import" | "filters" | null;

/** Full bookmarks management page: toolbar + filter rail + card list, with
 *  create/edit/import/manage dialogs, multi-select, and batch operations. */
export function BookmarksPage() {
  const state = useBookmarksPage();

  const [dialog, setDialog] = useState<DialogKind>(null);
  const [manageTab, setManageTab] = useState<ManageTab>("categories");
  const [editing, setEditing] = useState<BookmarkDTO | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [search, setSearch] = useState("");
  const [batchBusy, setBatchBusy] = useState(false);
  const { t } = useI18n();

  // ---- Multi-select state ----
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());

  const isSelected = useCallback(
    (id: number) => selectedIds.has(id),
    [selectedIds],
  );

  const toggleSelect = useCallback((id: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const selectAll = useCallback(() => {
    setSelectedIds(new Set(state.bookmarks.map((b) => b.id)));
  }, [state.bookmarks]);

  const clearSelection = useCallback(() => {
    setSelectedIds(new Set());
  }, []);

  // Clear selection when filters/page change (avoid acting on invisible items)
  const patchFilters = (patch: Partial<BookmarksFilters>) => {
    state.setFilters((f) => ({ ...f, ...patch }));
    state.setPage(1);
    setSelectedIds(new Set());
  };

  const onSearch = (value: string) => {
    setSearch(value);
    patchFilters({ q: value });
  };

  // The categories rail no longer has an "All" entry. Once categories are
  // available, default to the first visible category; if the selected category
  // later disappears, fall back to the new first category.
  useEffect(() => {
    if (state.categories.length === 0) return;
    const selectedExists = state.filters.categoryId
      ? state.categories.some((category) => category.id === state.filters.categoryId)
      : false;
    if (!selectedExists) {
      state.setFilters((filters) => ({ ...filters, categoryId: state.categories[0].id }));
      state.setPage(1);
      clearSelection();
    }
  }, [state.categories, state.filters.categoryId, state.setFilters, state.setPage, clearSelection]);

  const openCreate = () => {
    setEditing(null);
    setDialog("create");
  };

  const openEdit = (b: BookmarkDTO) => {
    setEditing(b);
    setDialog("edit");
  };

  const submitForm = async (value: BookmarkFormValue) => {
    setSubmitting(true);
    try {
      const params = {
        url: value.url,
        title: value.title,
        description: value.description || undefined,
        categoryId: value.categoryId || undefined,
        tagIds: value.tagIds,
        pinned: value.pinned,
        favorite: value.favorite,
      };
      if (editing) {
        await state.updateBookmark(editing.id, params);
      } else {
        await state.createBookmark(params);
      }
      setDialog(null);
      setEditing(null);
    } finally {
      setSubmitting(false);
    }
  };

  const editInitial = useMemo(
    () => (editing ? bookmarkToForm(editing) : EMPTY_BOOKMARK_FORM),
    [editing],
  );

  // ---- Batch operations ----
  const ids = useMemo(() => [...selectedIds], [selectedIds]);

  const runBatch = async (fn: (ids: number[]) => Promise<void>) => {
    if (ids.length === 0) return;
    setBatchBusy(true);
    try {
      await fn(ids);
      clearSelection();
    } finally {
      setBatchBusy(false);
    }
  };

  const currentStatus = state.filters.status;

  const hasSelection = selectedIds.size > 0;
  // Selection is page-scoped (cleared on every filter/page change), so the
  // visible page is the universe for both the count and the select-all toggle.
  const visibleCount = state.bookmarks.length;
  const allVisibleSelected =
    visibleCount > 0 && state.bookmarks.every((b) => selectedIds.has(b.id));

  const dialogTitle = editing ? t("bookmarks.editBookmark") : t("bookmarks.newBookmark");
  const dialogIcon = editing ? <BookmarkIcon /> : <PlusIcon />;

  const pages = Math.max(1, Math.ceil(state.total / state.pageSize));

  const goPage = (p: number) => {
    clearSelection();
    state.setPage(p);
  };

  return (
    <div className={styles.page}>
      <main className={styles.main}>
        <header className={styles.pageHead}>
          <div className={styles.headLeft}>
            <div>
              <h1 className={styles.title}>
                <LayersIcon className={styles.titleIcon} />
                {t("bookmarks.title")}
              </h1>
              <p className={styles.subtitle}>
                {state.total} {state.total === 1 ? t("bookmarks.bookmark") : t("bookmarks.bookmarks")}
                {state.filters.status !== "active" ? ` · ${state.filters.status === "trash" ? t("filters.trash") : state.filters.status === "archived" ? t("filters.archived") : state.filters.status}` : ""}
              </p>
            </div>
          </div>
          <div className={styles.headActions}>
            <Button
              variant="ghost"
              size="sm"
              icon={<FolderIcon />}
              onClick={() => {
                setManageTab("categories");
                setDialog("manage");
              }}
            >
              <span className={styles.btnLabel}>{t("bookmarks.categories")}</span>
            </Button>
            <Button
              variant="ghost"
              size="sm"
              icon={<TagIcon />}
              onClick={() => {
                setManageTab("tags");
                setDialog("manage");
              }}
            >
              <span className={styles.btnLabel}>{t("bookmarks.tags")}</span>
            </Button>
            <Button
              variant="secondary"
              size="sm"
              icon={<UploadIcon />}
              onClick={() => setDialog("import")}
            >
              {t("common.import")}
            </Button>
            <Button variant="primary" size="sm" icon={<PlusIcon />} onClick={openCreate}>
              <span className={styles.btnLabel}>{t("bookmarks.add")}</span>
            </Button>
          </div>
        </header>

        <div className={styles.toolbar}>
          <div className={styles.search}>
            <SearchIcon className={styles.searchIcon} />
            <input
              type="search"
              className={styles.searchInput}
              placeholder={t("bookmarks.searchPlaceholder")}
              value={search}
              onChange={(e) => onSearch(e.target.value)}
            />
          </div>
          <IconButton
            label={t("common.filters")}
            className={styles.filterToggle}
            onClick={() => setDialog("filters")}
          >
            <FilterIcon />
          </IconButton>
        </div>

        <div className={styles.layout}>
          <aside className={styles.rail}>
            <FilterSidebar
              filters={state.filters}
              categories={state.categories}
              tags={state.tags}
              onChange={patchFilters}
            />
          </aside>

          <section className={cn(styles.listPanel, state.loading && styles.listColLoading)}>
            {/* ---- Batch action bar (stable, Select-all always reachable) ----
                Kept inside the right list panel so the bookmarks and read-later
                layouts begin on the same baseline. */}
            <div className={styles.batchBar}>
              <span className={styles.batchCount}>
                {t("bookmarks.selectedOf", {
                  selected: selectedIds.size,
                  total: visibleCount,
                })}
              </span>

              <div className={styles.batchControls}>
                <button
                  type="button"
                  className={styles.batchLink}
                  onClick={allVisibleSelected ? clearSelection : selectAll}
                  disabled={visibleCount === 0}
                >
                  {allVisibleSelected
                    ? t("bookmarks.deselectAll")
                    : t("bookmarks.selectAll")}
                </button>
                <button
                  type="button"
                  className={cn(styles.batchLink, styles.batchClear)}
                  onClick={clearSelection}
                  disabled={!hasSelection}
                >
                  {t("bookmarks.clearSelection")}
                </button>
              </div>

              <div className={styles.batchActions}>
                {hasSelection ? (
                  <>
                    {currentStatus === "trash" || currentStatus === "archived" ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        icon={<RestoreIcon />}
                        disabled={batchBusy}
                        onClick={() => void runBatch(state.batchRestore)}
                      >
                        <span className={styles.batchBtnLabel}>
                          {t("bookmarks.batchRestore")}
                        </span>
                      </Button>
                    ) : (
                      <Button
                        size="sm"
                        variant="ghost"
                        icon={<ArchiveIcon />}
                        disabled={batchBusy}
                        onClick={() => void runBatch(state.batchArchive)}
                      >
                        <span className={styles.batchBtnLabel}>
                          {t("bookmarks.batchArchive")}
                        </span>
                      </Button>
                    )}
                    {currentStatus !== "trash" ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        icon={<TrashIcon />}
                        disabled={batchBusy}
                        onClick={() => void runBatch(state.batchRemove)}
                      >
                        <span className={styles.batchBtnLabel}>
                          {t("bookmarks.batchTrash")}
                        </span>
                      </Button>
                    ) : null}
                    {batchBusy ? (
                      <span className={styles.batchBusy}>{t("bookmarks.batchBusy")}</span>
                    ) : null}
                  </>
                ) : null}
              </div>
            </div>

            {state.error ? (
              <div className={styles.banner} role="alert">{state.error}</div>
            ) : null}
            {state.actionError ? (
              <div className={styles.banner} role="alert">{state.actionError}</div>
            ) : null}

            {state.loading ? (
              <div className={styles.skeletonList}>
                {Array.from({ length: 6 }).map((_, i) => (
                  <GlassSkeleton key={i} lines={1} />
                ))}
              </div>
            ) : state.bookmarks.length === 0 ? (
              <EmptyState
                icon={<BookmarkIcon />}
                title={
                  state.filters.q ||
                  state.filters.categoryId ||
                  state.filters.tagId ||
                  state.filters.favorite ||
                  state.filters.pinned
                    ? t("common.noMatches")
                    : state.filters.status === "trash"
                      ? t("bookmarks.trashEmpty")
                      : state.filters.status === "archived"
                        ? t("bookmarks.nothingArchived")
                        : t("bookmarks.noBookmarks")
                }
                description={
                  state.filters.q ||
                  state.filters.categoryId ||
                  state.filters.tagId ||
                  state.filters.favorite ||
                  state.filters.pinned
                    ? t("common.clearFilters")
                    : t("bookmarks.noBookmarksDesc")
                }
                action={
                  state.bookmarks.length === 0 &&
                  !state.filters.q &&
                  state.filters.status === "active" ? (
                    <Button variant="primary" icon={<UploadIcon />} onClick={() => setDialog("import")}>
                      {t("bookmarks.importBookmarks")}
                    </Button>
                  ) : null
                }
              />
            ) : (
              <>
                <div className={styles.list}>
                  {state.bookmarks.map((b) => (
                    <BookmarkCard
                      key={b.id}
                      bookmark={b}
                      selectable
                      selected={isSelected(b.id)}
                      onToggleSelect={() => toggleSelect(b.id)}
                      onEdit={() => openEdit(b)}
                      onDelete={() => void state.removeBookmark(b.id)}
                      onArchive={() => void state.archiveBookmark(b.id)}
                      onRestore={() => void state.restoreBookmark(b.id)}
                      onOpen={() => void state.openBookmark(b.id)}
                    />
                  ))}
                </div>

                {pages > 1 ? (
                  <div className={styles.pagination}>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={state.page <= 1}
                      onClick={() => goPage(state.page - 1)}
                    >
                      {t("common.previous")}
                    </Button>
                    <span className={styles.pageInfo}>
                      {t("common.pageOf", { page: state.page, pages })}
                    </span>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={state.page >= pages}
                      onClick={() => goPage(state.page + 1)}
                    >
                      {t("common.next")}
                    </Button>
                  </div>
                ) : null}
              </>
            )}
          </section>
        </div>
      </main>

      {/* ---- Create / Edit bookmark ---- */}
      <Modal
        open={dialog === "create" || dialog === "edit"}
        onClose={() => {
          setDialog(null);
          setEditing(null);
        }}
        title={dialogTitle}
        icon={dialogIcon}
      >
        <BookmarkForm
          initial={editInitial}
          categories={state.categories}
          tags={state.tags}
          submitting={submitting}
          onSubmit={submitForm}
          onCancel={() => {
            setDialog(null);
            setEditing(null);
          }}
        />
      </Modal>

      {/* ---- Manage categories / tags ---- */}
      <Modal
        open={dialog === "manage"}
        onClose={() => setDialog(null)}
        title={t("bookmarks.manage")}
        description={t("bookmarks.manageDesc")}
      >
        <div className={styles.manageTabs} role="tablist" aria-label="Manage">
          <button
            type="button"
            role="tab"
            aria-selected={manageTab === "categories"}
            className={cn(styles.manageTab, manageTab === "categories" && styles.manageTabActive)}
            onClick={() => setManageTab("categories")}
          >
            <FolderIcon className={styles.manageTabIcon} />
            {t("bookmarks.categories")}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={manageTab === "tags"}
            className={cn(styles.manageTab, manageTab === "tags" && styles.manageTabActive)}
            onClick={() => setManageTab("tags")}
          >
            <TagIcon className={styles.manageTabIcon} />
            {t("bookmarks.tags")}
          </button>
        </div>
        {manageTab === "categories" ? (
          <CategoryManager
            categories={state.categories}
            busy={state.loading}
            onCreate={state.createCategory}
            onUpdate={state.updateCategory}
            onDelete={state.removeCategory}
          />
        ) : (
          <TagManager
            tags={state.tags}
            busy={state.loading}
            onCreate={state.createTag}
            onUpdate={state.updateTag}
            onDelete={state.removeTag}
          />
        )}
      </Modal>

      {/* ---- Filters sheet (mobile) ---- */}
      <Modal
        open={dialog === "filters"}
        onClose={() => setDialog(null)}
        title={t("common.filters")}
        icon={<FilterIcon />}
      >
        <FilterSidebar
          filters={state.filters}
          categories={state.categories}
          tags={state.tags}
          onChange={patchFilters}
        />
      </Modal>

      {/* ---- Import ---- */}
      <ImportDialog
        open={dialog === "import"}
        onClose={() => setDialog(null)}
        onImported={() => void state.reloadAll()}
      />
    </div>
  );
}
