# Homepage — Frontend (`web/`)

A React + TypeScript + Vite single-page app: the homepage / new-tab UI with a
refined glassmorphism aesthetic over a wallpaper background. The homepage,
bookmarks, and read-later management surfaces are wired to the Go backend; the
home feature still ships mock data as a component-default reference.

## Stack

- **React 18** + **TypeScript** (strict) + **Vite 5**
- **react-router-dom** for routing
- **Hand-written CSS** with design tokens (`src/styles/theme.css`) and
  **CSS Modules** per component — no UI/component library, no Tailwind, so the
  glass aesthetic stays fully under control and the bundle stays small.
- **Inline SVG icons** (`src/components/icons.tsx`) — zero icon dependency.

## Layout

```
web/
  index.html               # Vite entry; loads Inter (system fallback) + #root
  vite.config.ts           # /api proxy -> Go backend (dev), build config
  src/
    main.tsx               # React root; imports global styles
    app/AppShell.tsx       # wallpaper background + overlay + <Routes>
    pages/                 # HomePage, LoginPage, BookmarksPage, ReadLaterPage,
                           #   ComingSoon (settings), NotFound
    components/            # GlassPanel, SearchBar, Clock, QuickLinks,
                           #   BookmarkGroup, ReadLaterPreview, Widgets,
                           #   TopBar, BottomNav, WallpaperSwitcher, SectionHeader,
                           #   Modal, ui (Button/FormField/...), GlassSkeleton, EmptyState
    features/
      auth/AuthProvider.tsx  # session state, login/setup/logout, route guard
      home/                 # mock data + API mappers + useHomeData hook
      bookmarks/            # BookmarksPage feature: hooks, forms, import dialog,
      readLater/            # ReadLaterPage feature: hooks, filters, card, form
    lib/
      api/                  # client.ts (envelope/CSRF/401/retry), csrf.ts,
                            #   auth.ts, bookmarks.ts, readLater.ts
      cn.ts, types.ts, useClock.ts, wallpapers.ts
    styles/                # theme.css (tokens) + global.css (reset/base/bg)
  public/wallpaper/        # web-friendly wallpaper copies (see scripts below)
  scripts/  (../scripts)   # gen-wallpapers.py regenerates the public copies
```

## Getting started

```bash
cd web
pnpm install        # or: npm install
pnpm dev            # http://localhost:5173  (proxies /api -> :8080)
```

> The dev server needs a running Go backend on `:8080` for `/api` calls (start
> it with `make run` in the repo root). On hosts where `inotify` is limited
> (some containers/sandboxes), start dev with `HOMEPAGE_DEV_POLL=1 pnpm dev`
> to use file polling for HMR instead.

## Scripts

| Script            | Description                                  |
| ----------------- | -------------------------------------------- |
| `pnpm dev`        | Vite dev server with HMR + `/api` proxy      |
| `pnpm build`      | `tsc --noEmit` typecheck + production build  |
| `pnpm preview`    | Serve the production build locally           |
| `pnpm typecheck`  | TypeScript typecheck only (`tsc --noEmit`)   |

## Wallpapers

The source art lives in the repo root `wallpaper/` and is **very large** (some
> 40 MB). Never reference those directly. Web-friendly copies are generated into
`public/wallpaper/` by:

```bash
# from the repo root
python3 scripts/gen-wallpapers.py
```

This emits a full hero (`<slug>.jpg`, max 2400px wide) and a picker thumbnail
(`<slug>-thumb.jpg`) per source. The frontend registry is
`src/lib/wallpapers.ts`; add a wallpaper there after (re)generating.

## Design tokens

All colour, type, spacing, radius, shadow, motion, and z-index values live in
`src/styles/theme.css`. Tune the look of the whole app from there. The accent
"aurora" gradient (`--accent-grad`) is used sparingly for focus/active states.

## API client & auth

The app talks to the Go backend through a thin, dependency-free client in
`src/lib/api/`:

- **`client.ts`** — the single `request<T>()` primitive. It always sends
  cookies (`credentials: 'include'`), JSON-encodes bodies, unwraps the
  `{ data, error, requestId }` envelope, and maps failures to a typed
  `ApiError` (status + machine code). On **401** it clears auth state via a
  registered handler; on **403 `CSRF_INVALID`** it refreshes the CSRF token
  once and retries the unsafe request.
- **`csrf.ts`** — reads the readable `homepage_csrf` cookie and echoes it as
  `X-CSRF-Token` on unsafe methods; `refreshCSRFToken()` hits
  `GET /api/v1/auth/csrf`.
- **`auth.ts` / `bookmarks.ts` / `readLater.ts`** — typed wrappers over the
  `request` primitive, one per resource.

Auth state lives in `features/auth/AuthProvider.tsx` (React context + `useAuth`
hook). On mount it probes `GET /api/v1/auth/me`; `RequireAuth` redirects
unauthenticated users to `/login`. The setup tab is visible in Vite development
(or when built with `VITE_ENABLE_SETUP=true`) for local initialization only;
production builds hide it so public deployments expose sign-in but not
registration. Logout is wired into the top bar.

Loading uses glass skeletons and empty data shows tasteful empty states, so the
premium feel is preserved while real data loads.

## Bookmarks management (`/bookmarks`)

A full management surface (not a plain admin table) wired to the backend
bookmark/category/tag/import APIs:

- **Layout**: PC shows a left filter rail (status tabs, categories, tag cloud,
  quick toggles) + a card list with a search toolbar. On mobile the rail
  collapses into a filter sheet and the cards go single-column; no horizontal
  overflow.
- **Bookmarks**: create/edit (URL, title, description, category, tags, pinned,
  favorite), archive, move to trash, restore from trash, and open tracking.
  Duplicate URLs surface the backend's `409` as a friendly error.
- **Categories & tags**: inline CRUD in a glass modal (create, rename, archive/
  delete). Reorder is structurally reserved (`sortOrder`) but not yet driven.
- **Search & filters**: `q` free-text, `categoryId`, `tagId`, `status`
  (active/archived/trash), `favorite`, `pinned`, plus pagination.
- **Import**: drag-and-drop or paste a browser bookmarks HTML export, choose a
  duplicate policy (skip/update/duplicate), and see a detailed result summary
  (created/updated/skipped/failed counters + bounded sample reason lines).
  The list refreshes after a successful import.
- The API client (`bookmarks.ts`) now covers every category/tag/bookmark CRUD
  endpoint plus multipart HTML import; the client was extended with a `form`
  option so multipart uploads keep the browser-set boundary.

Structure: `features/bookmarks/` owns `useBookmarksPage` (data + mutations +
filters), the forms, the filter rail, the card, and the import dialog; the page
in `pages/BookmarksPage.tsx` composes them and stays presentational.

## Read-later management (`/read-later`)

A full management surface for the reading queue, wired to the backend
read-later API (`lib/api/readLater.ts`) and the shared tags API:

- **States**: a reading queue (unread + reading) is the default view; archived
  and trash are opt-in via the Status rail. `open` records a read and moves
  unread → reading; `restore` returns trash/archived items to unread; `archive`
  hides an item from the default queue; `delete` is a soft-delete to trash.
- **Items**: create/edit (URL, title, excerpt/notes, author, site name, source,
  reading time, priority, favorite, tags); favorite toggle, archive, restore,
  and trash inline. Opening a link records the read best-effort and still opens
  the URL even if the recording fails. Duplicate URLs surface the backend's
  `409` as a friendly inline error on the URL field.
- **Search & filters**: `q` free-text, `state` (queue/unread/reading/archived/
  trash), `tagId`, `domain` (click a card's domain to filter), `favorite`,
  `priority` chips, plus sort (newest/oldest/recently updated/priority/
  title) and pagination.
- **Tags**: reused from the bookmarks feature — the same `tags` table backs
  both, so the tag manager is shared and tag selection is available in the form.

Structure: `features/readLater/` owns `useReadLaterPage` (data + mutations +
filters), the filter rail, the card, and the form; `pages/ReadLaterPage.tsx`
composes them and reuses `features/bookmarks/TagManager` for tag CRUD.

## Notes & next steps

- **Live by default**: the homepage and bookmarks management read real data.
  `features/home/mockData.ts` remains as a component-default reference but is
  no longer the data source.
- **Routing**: `/` (guarded), `/login` (public), `/bookmarks` (full management,
  guarded), `/read-later` (full management, guarded), and reserved `/settings`
  (ComingSoon placeholder).
- **Fonts**: Inter is loaded from Google Fonts with a full system fallback;
  remove the `<link>` in `index.html` to go fully offline/self-hosted.
