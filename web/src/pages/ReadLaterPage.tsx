import { useMemo, useState } from "react";
import { Button, IconButton } from "../components/ui";
import { Modal } from "../components/Modal";
import { EmptyState } from "../components/EmptyState";
import { GlassSkeleton } from "../components/GlassSkeleton";
import {
  PlusIcon,
  BookIcon,
  TagIcon,
  FilterIcon,
  SearchIcon,
  InboxIcon,
  ArchiveIcon,
  TrashIcon,
} from "../components/icons";
import { TagManager } from "../features/bookmarks/TagManager";
import { useReadLaterPage } from "../features/readLater/useReadLaterPage";
import type { ReadLaterFilters as RLFilters } from "../features/readLater/useReadLaterPage";
import { ReadLaterFilters } from "../features/readLater/ReadLaterFilters";
import { ReadLaterCard } from "../features/readLater/ReadLaterCard";
import {
  ReadLaterForm,
  readLaterToForm,
  EMPTY_READ_LATER_FORM,
  type ReadLaterFormValue,
} from "../features/readLater/ReadLaterForm";
import type { ReadLaterDTO } from "../lib/api/readLater";
import { ApiError } from "../lib/api/client";
import { useI18n } from "../i18n/I18nProvider";
import styles from "./ReadLaterPage.module.css";

type DialogKind = "create" | "edit" | "manage" | "filters" | null;

/** Full read-later management page: toolbar + filter rail + card list, with
 *  create/edit + tag-management dialogs. Responsive: the rail collapses into
 *  a filter sheet on mobile and actions stay reachable. */
export function ReadLaterPage() {
  const state = useReadLaterPage();

  const [dialog, setDialog] = useState<DialogKind>(null);
  const [editing, setEditing] = useState<ReadLaterDTO | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [search, setSearch] = useState("");
  const [duplicateError, setDuplicateError] = useState<string | null>(null);
  const { t } = useI18n();

  const stateLabel = (s: RLFilters["state"]): string => {
    switch (s) {
      case "queue": return t("readlater.readingQueue");
      case "unread": return t("readlater.unread");
      case "reading": return t("readlater.reading");
      case "archived": return t("readlater.archived");
      case "trash": return t("readlater.trash");
    }
  };

  // Apply a filter patch and reset to page 1 so the new result set shows
  // from its first page.
  const patchFilters = (patch: Partial<RLFilters>) => {
    state.setFilters((f) => ({ ...f, ...patch }));
    state.setPage(1);
  };

  const onSearch = (value: string) => {
    setSearch(value);
    patchFilters({ q: value });
  };

  const openCreate = () => {
    setEditing(null);
    setDuplicateError(null);
    setDialog("create");
  };

  const openEdit = (item: ReadLaterDTO) => {
    setEditing(item);
    setDuplicateError(null);
    setDialog("edit");
  };

  const submitForm = async (value: ReadLaterFormValue) => {
    setSubmitting(true);
    setDuplicateError(null);
    try {
      const params = {
        url: value.url,
        title: value.title,
        excerpt: value.excerpt,
        author: value.author,
        siteName: value.siteName,
        source: value.source,
        readingTimeMinutes: value.readingTimeMinutes,
        priority: value.priority,
        favorite: value.favorite,
        tagIds: value.tagIds,
      };
      if (editing) {
        await state.updateItem(editing.id, params);
      } else {
        await state.createItem(params);
      }
      setDialog(null);
      setEditing(null);
    } catch (err) {
      // mutate() already surfaced a generic actionError; promote a duplicate
      // URL (409) to a friendly inline message on the URL field.
      if (err instanceof ApiError && err.status === 409) {
        setDuplicateError(t("readlater.duplicateError"));
      }
    } finally {
      setSubmitting(false);
    }
  };

  const editInitial = useMemo(
    () => (editing ? readLaterToForm(editing) : EMPTY_READ_LATER_FORM),
    [editing],
  );

  // Distinct priorities in the current view → drive the priority filter chips.
  const priorities = useMemo(() => {
    const set = new Set<number>();
    for (const it of state.items) if (it.priority && it.priority > 0) set.add(it.priority);
    return [...set].sort((a, b) => b - a);
  }, [state.items]);

  const dialogTitle = editing ? t("readlater.editItem") : t("readlater.newItem");
  const dialogIcon = editing ? <BookIcon /> : <PlusIcon />;
  const pages = Math.max(1, Math.ceil(state.total / state.pageSize));
  const hasFilters =
    !!state.filters.q ||
    !!state.filters.tagId ||
    !!state.filters.domain ||
    state.filters.favorite ||
    state.filters.priority !== null;

  return (
    <div className={styles.page}>
      <main className={styles.main}>
        <header className={styles.pageHead}>
          <div className={styles.headLeft}>
            <div>
              <h1 className={styles.title}>
                <BookIcon className={styles.titleIcon} />
                {t("readlater.title")}
              </h1>
              <p className={styles.subtitle}>
                {state.total} {state.total === 1 ? t("common.items", { count: 1 }) : t("common.items_plural", { count: state.total })} · {stateLabel(state.filters.state)}
              </p>
            </div>
          </div>
          <div className={styles.headActions}>
            <Button
              variant="ghost"
              size="sm"
              icon={<TagIcon />}
              onClick={() => setDialog("manage")}
            >
              <span className={styles.btnLabel}>{t("readlater.tags")}</span>
            </Button>
            <Button variant="primary" size="sm" icon={<PlusIcon />} onClick={openCreate}>
              <span className={styles.btnLabel}>{t("readlater.add")}</span>
            </Button>
          </div>
        </header>

        <div className={styles.toolbar}>
          <div className={styles.search}>
            <SearchIcon className={styles.searchIcon} />
            <input
              type="search"
              className={styles.searchInput}
              placeholder={t("readlater.searchPlaceholder")}
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
            <ReadLaterFilters
              filters={state.filters}
              tags={state.tags}
              total={state.total}
              priorities={priorities}
              onChange={patchFilters}
            />
          </aside>

          <section className={styles.listCol}>
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
            ) : state.items.length === 0 ? (
              <EmptyState
                icon={state.filters.state === "archived" ? <ArchiveIcon /> : state.filters.state === "trash" ? <TrashIcon /> : <InboxIcon />}
                title={
                  hasFilters
                    ? t("common.noMatches")
                    : state.filters.state === "trash"
                      ? t("readlater.emptyTrash")
                      : state.filters.state === "archived"
                        ? t("readlater.emptyArchived")
                        : t("readlater.emptyQueue")
                }
                description={
                  hasFilters
                    ? t("common.clearFilters")
                    : state.filters.state === "trash" || state.filters.state === "archived"
                      ? t("readlater.emptyTrashDesc")
                      : t("readlater.emptyQueueDesc")
                }
                action={
                  !hasFilters && state.filters.state === "queue" ? (
                    <Button variant="primary" icon={<PlusIcon />} onClick={openCreate}>
                      {t("readlater.addLink")}
                    </Button>
                  ) : null
                }
              />
            ) : (
              <>
                <div className={styles.list}>
                  {state.items.map((item) => (
                    <ReadLaterCard
                      key={item.id}
                      item={item}
                      onOpen={() => void state.openItem(item.id)}
                      onEdit={() => openEdit(item)}
                      onDelete={() => void state.removeItem(item.id)}
                      onArchive={() => void state.archiveItem(item.id)}
                      onRestore={() => void state.restoreItem(item.id)}
                      onPurge={() => void state.purgeItem(item.id)}
                      onToggleFavorite={() =>
                        void state.updateItem(item.id, { favorite: !item.favorite })
                      }
                      onFilterDomain={(domain) => patchFilters({ domain })}
                    />
                  ))}
                </div>

                {pages > 1 ? (
                  <div className={styles.pagination}>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={state.page <= 1}
                      onClick={() => state.setPage(state.page - 1)}
                    >
                      Previous
                    </Button>
                    <span className={styles.pageInfo}>
                      Page {state.page} of {pages}
                    </span>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={state.page >= pages}
                      onClick={() => state.setPage(state.page + 1)}
                    >
                      Next
                    </Button>
                  </div>
                ) : null}
              </>
            )}
          </section>
        </div>
      </main>

      {/* ---- Create / Edit item ---- */}
      <Modal
        open={dialog === "create" || dialog === "edit"}
        onClose={() => {
          setDialog(null);
          setEditing(null);
          setDuplicateError(null);
        }}
        title={dialogTitle}
        icon={dialogIcon}
      >
        <ReadLaterForm
          initial={editInitial}
          tags={state.tags}
          submitting={submitting}
          duplicateError={duplicateError}
          onSubmit={submitForm}
          onCancel={() => {
            setDialog(null);
            setEditing(null);
            setDuplicateError(null);
          }}
        />
      </Modal>

      {/* ---- Manage tags (shared tags table, reused from bookmarks) ---- */}
      <Modal
        open={dialog === "manage"}
        onClose={() => setDialog(null)}
        title={t("readlater.tags")}
        description={t("bookmarks.manageDesc")}
        icon={<TagIcon />}
      >
        <TagManager
          tags={state.tags}
          busy={state.loading}
          onCreate={state.createTag}
          onUpdate={state.updateTag}
          onDelete={state.removeTag}
        />
      </Modal>

      {/* ---- Filters sheet (mobile) ---- */}
      <Modal
        open={dialog === "filters"}
        onClose={() => setDialog(null)}
        title={t("common.filters")}
        icon={<FilterIcon />}
      >
        <ReadLaterFilters
          filters={state.filters}
          tags={state.tags}
          total={state.total}
          priorities={priorities}
          onChange={patchFilters}
        />
      </Modal>
    </div>
  );
}
