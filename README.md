# homepage

A personal browser homepage / new-tab page: bookmarks, read-later, and a
polished responsive UI, backed by a small Go API and SQLite. Single-user,
self-hosted, private.

This repository currently contains **phase 1: the Go backend skeleton** — a
stable, well-structured foundation that later phases (auth, bookmarks,
read-later, import/export, frontend) build on.

> The full product/UI/data-model/API plan lives with the planning task; this
> README documents the implementation as it exists today.

## Goals for the skeleton

- **Stable and elegant** over clever: standard library where it fits, one
  well-chosen dependency (SQLite), forward-only migrations.
- **Structured and observable** from day one: typed config, `log/slog`
  structured logging, per-request ids, uniform JSON envelope, health check.
- **Clear module boundaries** so auth, bookmarks, and import slot in without
  reshuffling the tree.

## Tech choices (and why)

| Concern        | Choice                                   | Why                                                  |
| -------------- | ---------------------------------------- | ---------------------------------------------------- |
| Language       | Go 1.22+ (built on 1.25)                 | Single-binary deploy, great stdlib                   |
| HTTP routing   | `net/http` ServeMux (method + patterns)  | No extra dep; enough for this scope                  |
| Logging        | `log/slog`                               | Stdlib, structured, no dep                           |
| Database       | SQLite via `modernc.org/sqlite`          | Pure-Go (no CGO), easy cross-build, single file      |
| Passwords      | bcrypt via `golang.org/x/crypto`         | Quasi-stdlib, the one crypto dep we accept           |
| Migrations     | Embedded `.sql` + tiny in-process runner | No external tool; idempotent; forward-only           |

The non-stdlib dependencies are deliberately minimal: the SQLite driver,
`golang.org/x/crypto` (for bcrypt), and `golang.org/x/net/html` (for tolerant
browser bookmark HTML parsing). That is intentional per the "keep it lean"
guidance; revisit each addition deliberately.

## Project layout

```
cmd/
  server/main.go            # entry point: config -> log -> db -> migrate -> http
internal/
  api/                      # uniform response envelope + typed error model
  auth/                     # authentication: store/service/handlers/middleware
  bookmark/                 # bookmarks core: categories/tags/bookmarks CRUD + search
                            #  + Netscape HTML import (import_html.go / handler_import.go)
  readlater/                # read-later items CRUD/state/search/tag association
  config/                   # env-driven configuration + validation
  csrf/                     # double-submit CSRF cookie/header middleware
  db/                       # SQLite open (WAL/FK/busy_timeout) + migration runner
  httpx/                    # ClientIPResolver: trusted-proxy-aware client IP
  logging/                  # slog logger factory
  ratelimit/                # in-memory login limiter (Limiter interface)
  reqid/                    # request-id context propagation (dep-free)
  security/                 # password hashing (bcrypt) + session token gen/hash
  server/
    server.go               # router wiring, middleware chain, graceful shutdown
    handler/health.go       # GET /api/v1/system/health (+ /healthz alias)
    middleware/             # Recover, RequestID, Logger
scripts/
  gen-wallpapers.py         # generate web-friendly wallpaper copies for web/
migrations/
  embed.go                  # embeds all *.sql
  0001_init.sql             # users + sessions (auth data-model foundation)
  0002_bookmarks.sql        # categories, tags, bookmarks, bookmark_tags
  0003_read_later.sql       # read_later_items + read_later_tags
web/                        # React + TypeScript + Vite frontend (visual prototype)
  src/                      #   app shell, pages, components, lib, styles
  public/wallpaper/         #   web-friendly wallpaper copies (generated)
wallpaper/                  # source wallpaper art (very large; never served directly)
```

`internal/auth` is layered as **Store** (persistence interface + SQLite impl) →
**Service** (business rules) → **handlers** (HTTP), with `RequireAuth`
middleware exposing the protected-route pattern for upcoming modules.
`internal/httpx.ClientIPResolver` is the trusted-proxy-aware source of the
client IP used by login rate limiting. `internal/bookmark` and
`internal/readlater` follow the same Store → Service → handlers layering; every
endpoint is wrapped in `auth.RequireAuth` and every query is scoped by `user_id`.

Reserved for upcoming phases (not yet implemented, boundaries already clear):
`internal/category` (standalone), `internal/metadata`, `internal/backup` (JSON
backup/export), and the `web/` frontend. Protected routes wrap their handlers
with `auth.RequireAuth(svc)`.

## Configuration

All settings come from environment variables (see `.env.example`):

| Variable                  | Default                  | Notes                                        |
| ------------------------- | ------------------------ | -------------------------------------------- |
| `APP_ENV`                 | `development`            | `development` \| `production`                |
| `APP_BASE_URL`            | `http://localhost:8080`  | Canonical public origin (reserved)           |
| `APP_HTTP_ADDR`           | `:8080`                  | Listen address                               |
| `APP_READ_TIMEOUT`        | `15s`                    | Go duration                                  |
| `APP_WRITE_TIMEOUT`       | `15s`                    | Go duration                                  |
| `APP_LOG_LEVEL`           | `info`                   | debug\|info\|warn\|error                      |
| `APP_LOG_FORMAT`          | `text` (dev) / `json`    | text\|json                                    |
| `APP_DATABASE_PATH`       | `./data/homepage.db`     | Parent dir is auto-created                    |
| `APP_SESSION_SECRET`      | _(empty)_                | Required in production, ≥32 bytes (reserved) |
| `APP_SETUP_ENABLED`       | dev→true / prod→false    | One-time admin setup endpoint                |
| `APP_TRUSTED_PROXY_CIDRS` | _(empty)_                | Trusted reverse-proxy CIDRs (see below)     |
| `APP_SESSION_TTL`         | `720h`                   | Session lifetime                             |
| `APP_BCRYPT_COST`         | `12`                     | bcrypt cost (4–31)                           |
| `APP_COOKIE_NAME`         | `homepage_session`       | Session cookie name                          |
| `APP_COOKIE_SECURE`       | prod→true / dev→false    | Cookie Secure attribute                      |
| `APP_COOKIE_SAMESITE`     | `lax`                    | lax\|strict\|none (none requires Secure)     |
| `APP_CSRF_COOKIE_NAME`    | `homepage_csrf`          | Readable CSRF double-submit cookie           |
| `APP_CSRF_HEADER_NAME`    | `X-CSRF-Token`           | Header expected on unsafe methods            |
| `APP_LOGIN_MAX_FAILURES`  | `5`                      | Failed logins allowed before throttle        |
| `APP_LOGIN_WINDOW`        | `15m`                    | Sliding failure window                       |

## Run

```bash
cp .env.example .env        # optional, defaults work in development
make run                    # builds ./bin/server and runs it
```

Then:

```bash
curl -i http://localhost:8080/api/v1/system/health
# HTTP/1.1 200 OK
# X-Request-ID: req_...
# {"data":{"status":"ok","service":"homepage","time":"..."},"requestId":"req_..."}
```

Migrations run automatically on startup (creating `./data/homepage.db` and the
`schema_migrations`/`users`/`sessions` tables). The process shuts down cleanly
on `SIGINT`/`SIGTERM`.

## Authentication

Local first-run initialization, login, logout, and current-user are implemented
over server-side sessions stored by hash in SQLite. Passwords are bcrypt-
hashed and never logged; the raw session token lives only in the client
cookie; login responses are uniform so usernames cannot be enumerated. The
setup endpoint is enabled by default only in development; production disables
it by default so public deployments do not expose registration/initialization.

```bash
# 1) One-time admin setup (development/local only; disabled in production by default)
curl -i -c cookies.txt -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<strong>","displayName":"Admin"}' \
  http://localhost:8080/api/v1/auth/setup            # 201

# 2) Login (sets homepage_session + homepage_csrf cookies)
curl -i -c cookies.txt -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"<strong>"}' \
  http://localhost:8080/api/v1/auth/login             # 200

# 3) Current user (safe GET; requires only the session cookie)
curl -i -b cookies.txt http://localhost:8080/api/v1/auth/me   # 200

# 4) Logout (unsafe POST; requires session cookie + X-CSRF-Token header)
# Read homepage_csrf from cookies.txt in scripts, then echo it in the header.
curl -i -b cookies.txt -X POST -H "X-CSRF-Token: <homepage_csrf>" \
  http://localhost:8080/api/v1/auth/logout            # 200
```

Security notes:
- Public deployments should keep `APP_SETUP_ENABLED=false` and use a database
  initialized locally (or restored from a trusted backup), so `/login` only
  allows existing users to sign in.
- The session cookie is `HttpOnly`, `SameSite=Lax` (configurable), `Path=/`,
  and `Secure` in production. The CSRF cookie intentionally is **not**
  `HttpOnly` so browser JavaScript can echo it in `X-CSRF-Token`.
- Failed logins are rate-limited per `(ip, username)` in memory; a blocked key
  returns `429` with a `Retry-After` header. The `ratelimit.Limiter` interface
  is the swap point for a persistent/distributed backend later.
- **Real client IP behind a reverse proxy is resolved safely** by
  `httpx.ClientIPResolver`: forwarded headers (`X-Forwarded-For`, `X-Real-IP`)
  are honoured ONLY when the TCP peer is inside `APP_TRUSTED_PROXY_CIDRS`. With
  that empty (the default) no proxy is trusted, so a direct client cannot forge
  a header to rotate its apparent IP and bypass per-IP rate limiting. For a
  public deployment, set `APP_TRUSTED_PROXY_CIDRS` to your proxy's CIDR and make
  sure the proxy overwrites `X-Forwarded-For`.
- Wrapping a future protected route:
  `mux.Handle("GET /api/v1/bookmarks", auth.RequireAuth(svc)(h))`, then read the
  user via `auth.UserFromContext(r.Context())`.
- **Public deployment / trusted proxy.** The app must NOT be reachable by a
  direct client with forged headers unless you intend it. Two safe setups:
  1. App listens on `127.0.0.1` / a private interface only, behind Caddy/Nginx
     that terminates TLS and sets `X-Forwarded-For`; set
     `APP_TRUSTED_PROXY_CIDRS` to the proxy's CIDR (e.g. `127.0.0.0/8`).
  2. App is directly exposed but you leave `APP_TRUSTED_PROXY_CIDRS` empty —
     then the real client IP is always the TCP peer and login rate limiting is
     correct, at the cost of less granular IPs when a proxy is later added.
## Frontend API Contract

Browser clients must use cookie credentials and CSRF headers consistently:

1. Call `POST /api/v1/auth/setup` or `POST /api/v1/auth/login` without a CSRF
   header. On success the server sets both `homepage_session` (HttpOnly) and
   `homepage_csrf` (readable) cookies.
2. If the frontend needs a token before or after login, call
   `GET /api/v1/auth/csrf`. It sets/refreshes `homepage_csrf` and returns:
   `{ "token": "...", "cookieName": "homepage_csrf", "headerName": "X-CSRF-Token" }`.
3. All browser API calls must include cookies: `fetch(..., { credentials:
   'include' })` or Axios `withCredentials: true`.
4. Safe methods (`GET`, `HEAD`, `OPTIONS`) do not need CSRF.
5. Unsafe methods (`POST`, `PATCH`, `PUT`, `DELETE`) with the session cookie
   must include `X-CSRF-Token` equal to the `homepage_csrf` cookie value. This
   includes logout, categories, tags, bookmarks, bookmark HTML import, and
   read-later mutations.
6. `401 UNAUTHORIZED` means the session is missing/expired/invalid and the UI
   should show login. `403 CSRF_INVALID` means refresh/re-read the CSRF token
   and retry once, then fall back to login if it still fails.

Example `fetch` helper:

```js
function cookie(name) {
  return document.cookie.split('; ').find(v => v.startsWith(name + '='))?.split('=')[1] ?? '';
}

async function api(path, options = {}) {
  const method = (options.method ?? 'GET').toUpperCase();
  const headers = new Headers(options.headers);
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method)) {
    headers.set('X-CSRF-Token', decodeURIComponent(cookie('homepage_csrf')));
  }
  return fetch('/api/v1' + path, { ...options, method, headers, credentials: 'include' });
}
```

CSRF implementation notes:
- The backend uses a double-submit cookie: unsafe requests are accepted only
  when the readable CSRF cookie and `X-CSRF-Token` header match exactly.
- `POST /api/v1/auth/setup` and `POST /api/v1/auth/login` are explicit CSRF
  exceptions so first-run/login can bootstrap the token. `POST /api/v1/auth/logout`
  is protected when a session cookie is present.
- CSRF cookie attributes mirror the session cookie for `Secure`, `SameSite`,
  `Path=/`, and session TTL; only `HttpOnly` differs (false for CSRF).
- The raw session token is never exposed to JavaScript; only the CSRF token is.

## Bookmarks

The bookmark core backend is implemented and **every** endpoint is protected
by `auth.RequireAuth`. Data is isolated per `user_id` (single-user today, but
the boundary is enforced on every query).

Endpoints:

| Method | Path                              | Notes                                  |
| ------ | --------------------------------- | -------------------------------------- |
| GET    | `/api/v1/categories`              | list                                   |
| POST   | `/api/v1/categories`              | create (name unique per user)          |
| GET    | `/api/v1/categories/{id}`         | detail                                 |
| PATCH  | `/api/v1/categories/{id}`         | update                                 |
| DELETE | `/api/v1/categories/{id}`         | soft delete (archived=1)               |
| GET    | `/api/v1/tags`                    | list                                   |
| POST   | `/api/v1/tags`                    | create (name unique per user)          |
| GET    | `/api/v1/tags/{id}`               | detail                                 |
| PATCH  | `/api/v1/tags/{id}`               | update                                 |
| DELETE | `/api/v1/tags/{id}`               | delete (cascade-removes tag links)     |
| GET    | `/api/v1/bookmarks`               | list/filter/search/paginate (see below) |
| POST   | `/api/v1/bookmarks`               | create (URL normalized; dup -> 409)    |
| GET    | `/api/v1/bookmarks/{id}`          | detail (includes tags)                 |
| PATCH  | `/api/v1/bookmarks/{id}`          | update (partial; tags replaced if set) |
| DELETE | `/api/v1/bookmarks/{id}`          | soft delete (status=trash)             |
| POST   | `/api/v1/bookmarks/{id}/restore`  | restore to active                      |
| POST   | `/api/v1/bookmarks/{id}/archive`  | move to archived                       |
| POST   | `/api/v1/bookmarks/{id}/open`     | record a click (count + last_opened_at) |

List query params: `q` (title/url/description/domain), `categoryId`, `tagId`,
`tag` (name), `status` (active|archived|trash; default active), `domain`,
`favorite`, `pinned`, `page`, `pageSize` (capped at 100), `sort`
(`created_desc` default; also `title`, `clicks`, `opened`, …). The list result
includes each item's tags.

Behavior notes:
- **Dedup:** a user cannot save the same normalized URL twice while it is not
  in the trash (HTTP 409). Normalization drops the root trailing slash and the
  default port, lowercases scheme/host, sorts query params, and drops the
  fragment. Trashing an item frees the normalized URL for re-use. Restoring a
  trashed item whose URL has since been taken by a new active bookmark also
  returns 409 (the conflict is detected, never a 500).
- **Soft delete:** `DELETE` sets `status='trash'` (never physical). Default list
  excludes trash; restore brings it back to active.
- **Multi-user isolation:** every query carries `user_id`; a second user cannot
  see another's categories/tags/bookmarks. Tag id ownership is checked on
  create/update so a stale tag id is dropped rather than failing the save.
  **categoryId is checked strictly**: a non-existent or foreign category id on
  create/update returns 400 (validated via the user-scoped lookup, so it can
  never produce an FK error or a cross-user association).
- **HTML import:** see the [Bookmark import](#bookmark-import) section —
  `POST /api/v1/bookmarks/import/html` parses Netscape exports (Chrome/Edge/Firefox).
  Metadata fetching remains a future task; the `metadata_status` column is ready.

## Read-later

The read-later backend is implemented and **every** endpoint is protected by
`auth.RequireAuth`. Data is isolated per `user_id` and tags reuse the shared
`tags` table via a dedicated `read_later_tags` join table.

Endpoints:

| Method | Path                                  | Notes                                  |
| ------ | ------------------------------------- | -------------------------------------- |
| GET    | `/api/v1/read-later`                  | list/filter/search/paginate            |
| POST   | `/api/v1/read-later`                  | create (URL normalized; dup -> 409)    |
| GET    | `/api/v1/read-later/{id}`             | detail (includes tags)                 |
| PATCH  | `/api/v1/read-later/{id}`             | update (partial; tags replaced if set) |
| DELETE | `/api/v1/read-later/{id}`             | soft delete (`state=trash`)            |
| POST   | `/api/v1/read-later/{id}/open`        | stamp `lastOpenedAt`; unread -> reading |
| POST   | `/api/v1/read-later/{id}/archive`     | move to archived                       |
| POST   | `/api/v1/read-later/{id}/restore`     | restore to unread                      |
| DELETE | `/api/v1/read-later/{id}/purge`       | hard-delete (only `state=trash`; 204)  |

Create body fields: `url` (required), `title`, `excerpt`, `author`, `siteName`,
`faviconUrl`, `coverImageUrl`, `readingTimeMinutes`, `priority`, `favorite`,
`source`, `tagIds`. Empty title falls back to the URL domain.

List query params: `q` (title/url/excerpt/domain/siteName), `state`
(`unread|reading|archived|trash`; default visible set is unread+reading),
`tagId`, `tag` (name), `domain`, `favorite`, `priority`, `page`, `pageSize`
(capped at 100), `sort` (`createdAtDesc` default; also `createdAtAsc`,
`updatedAtDesc`, `updatedAtAsc`, `priorityDesc`, `priorityAsc`, `titleAsc`).

Behavior notes:
- **Dedup:** a user cannot save the same normalized URL twice while neither row
  is trash (HTTP 409). Trashing an item frees the URL for re-use; restoring a
  trashed item whose URL has since been taken returns 409.
- **State flow:** new items start as `unread`; `open` records `lastOpenedAt` and
  automatically moves `unread` to `reading`; `archive` sets `archivedAt`; delete
  sets `deletedAt` and `state=trash`; restore clears archive/delete timestamps
  and returns the item to `unread`.
- **Tags:** `tagIds` must belong to the authenticated user; unknown/foreign tag
  ids return 400 rather than being silently accepted or leaking FK errors.
- **URL normalization:** read-later reuses the same conservative
  `bookmark.NormalizeURL` logic as bookmarks (lowercase host/scheme, drop
  default ports and fragments, sort query params).
- **Not included yet:** the optional `convert-to-bookmark` endpoint is deferred;
  it should be implemented as a small follow-up that calls bookmark service
  rules and maps duplicate bookmark URLs to 409.

## Bookmark import

`POST /api/v1/bookmarks/import/html` (behind `auth.RequireAuth`) imports a
Netscape-format bookmarks export as Chrome / Edge / Firefox produce. The body
is either:

- `multipart/form-data` with a `file` field (browser upload shape), or
- `application/json` `{ "html": "<...>" }` (handy for scripts/tests).

Optional query parameter `duplicateMode` selects the policy when a bookmark's
normalized URL already exists for the user:

| mode        | behavior                                                            |
| ----------- | ------------------------------------------------------------------ |
| `skip`      | (default) leave the existing bookmark; count the item as skipped   |
| `update`    | overwrite the existing bookmark's title/category with imported data |
| `duplicate` | try to add a second row; schema collisions are counted as skipped  |

Response:

```json
{
  "data": {
    "total": 7, "processed": 7,
    "created": 4, "skipped": 3, "updated": 0, "failed": 0,
    "duplicateMode": "skip",
    "samples": [{"kind":"skipped","url":"javascript:...","reason":"non-http(s) or invalid URL"}]
  }
}
```

Behavior:
- Folders (`<H3>`) become categories, **reused by name** (so re-importing is
  idempotent and cross-import safe). The document root title (`<H1>`, e.g.
  "Bookmarks bar") is NOT turned into a category — only real nested folders.
  A bookmark's category is its nearest enclosing folder; deeper folders are
  created too, but each bookmark is filed under its immediate parent.
- Only `http`/`https` URLs are imported; `javascript:`, `file:`, `data:`, and
  malformed URLs are skipped (counted, not failed).
- Empty titles fall back to the domain (same rule as single-bookmark create).
- Import reuses the same URL normalization + per-user dedup + category rules as
  the rest of the API, so an import can never bypass them.
- A single bad item never aborts the run; each item commits independently and
  is counted. Up to ~50 sample reason lines are returned (bounded response).
- Limits: max **10 MiB decoded HTML payload** and **20,000 processed items**.
  JSON allows a small envelope overhead but rejects raw JSON bodies beyond that
  envelope, or decoded `html` longer than 10 MiB, with `413 PAYLOAD_TOO_LARGE`;
  multipart files over 10 MiB also return `413`.
  `total` is the full parsed bookmark count, `processed` is the number actually
  evaluated, and `limitReached` is set when the 20,000-item cap truncates a run;
  unprocessed overflow items are counted as failed. The raw HTML is never
  persisted.
- No favicon downloading / metadata fetching in this phase — `favicon_url` is
  captured when the file carries one but the model field is reserved.

## Frontend

The UI is a React + TypeScript + Vite SPA in `web/` (visual prototype + UI
foundation; data is mocked, the API is not wired yet). It delivers a
high-fidelity glassmorphism homepage: wallpaper background, soft dark overlay,
live greeting/clock, a large glass search bar, quick-link tiles, bookmark group
previews, a read-later preview, and placeholder widgets, with responsive PC /
mobile layouts (single column + glass bottom nav on mobile).

```bash
cd web
pnpm install          # or: npm install
pnpm dev              # http://localhost:5173  (proxies /api -> :8080)
pnpm build            # typecheck + production build
pnpm preview          # serve the production build
```

Wallpapers: source art in `wallpaper/` is very large (some > 40 MB) and is
never served directly. Generate web-friendly copies into `web/public/wallpaper/`
with `python3 scripts/gen-wallpapers.py` (full hero + picker thumbnails). See
`web/README.md` for structure, design tokens, and API-wiring notes.

## Production deployment / Self-hosting

How to deploy the homepage on a public server with HTTPS, data persistence,
and automatic restarts.

### Architecture

```
Browser ──[HTTPS]──> Caddy (TLS) ──[HTTP]──> Go server (:8080)
                                              ├─ /api/*  (REST)
                                              ├─ /healthz
                                              └─ /*  (SPA static files)
                                              Data: /data/homepage.db (SQLite)
```

- **Caddy** terminates TLS (auto Let's Encrypt), adds security headers, and
  reverse-proxies to the Go server.
- **Go server** is a single static binary serving both API and the built React
  frontend from `APP_STATIC_DIR`.
- **SQLite** stores everything in a single file; the database directory is a
  Docker volume or bind mount — never inside the container.

### Quick start (Docker Compose)

```bash
# 1. Clone and prepare
git clone <repo-url> homepage && cd homepage

# 2. Create your environment file (REQUIRED: set APP_SESSION_SECRET)
cp .env.example .env
# Generate:  openssl rand -hex 32
# Also set:  APP_BASE_URL=https://your-domain.com

# 3. Initialize locally, then copy/restore the SQLite database to the server.
#    Public containers run with APP_SETUP_ENABLED=false, so there is no registration page.

# 4. Edit Caddyfile → replace "your-domain.example.com" with your real domain

# 5. Start and sign in with the locally initialized account
docker compose up -d
```

### Configuration reference

| Variable | Required | Default | Notes |
|----------|----------|---------|-------|
| `APP_ENV` | no | `development` | Set to `production` |
| `APP_BASE_URL` | no | `http://localhost:8080` | Public HTTPS origin, no trailing slash |
| `APP_HTTP_ADDR` | no | `:8080` | Listen address |
| `APP_SESSION_SECRET` | **yes** (prod) | — | ≥ 32 random bytes |
| `APP_DATABASE_PATH` | no | `./data/homepage.db` | SQLite file path |
| `APP_STATIC_DIR` | no | — | Path to React dist; Docker default `/srv/web` |
| `APP_TRUSTED_PROXY_CIDRS` | no | — | E.g. `172.16.0.0/12` for Docker bridge |
| `APP_SETUP_ENABLED` | no | `false` (prod) | Keep false on public servers; init locally |
| `APP_COOKIE_SECURE` | no | `true` (prod) | Must be true behind HTTPS |
| `APP_LOG_FORMAT` | no | `json` (prod) | Structured logging |

### Build from source (without Docker)

```bash
# Prerequisites: Go 1.25+, Node 22+, pnpm

cd web && pnpm install && pnpm build && cd ..
go build -o bin/server ./cmd/server

APP_ENV=production \
APP_STATIC_DIR=./web/dist \
APP_SETUP_ENABLED=false \
APP_SESSION_SECRET=$(openssl rand -hex 32) \
APP_BASE_URL=https://your-domain.com \
./bin/server
```

### Reverse proxy (Caddy / Nginx)

Point your proxy at `localhost:8080`.

**Upstream request headers** (proxy → backend; Caddy sets these automatically):
- `X-Forwarded-Proto: https`
- `X-Forwarded-For: <real-client-ip>` (rate limiting relies on this)

**Response security headers** (proxy → browser):
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`

Then set `APP_TRUSTED_PROXY_CIDRS` to the proxy's CIDR so the backend trusts
the forwarded headers. The compose file defaults to `172.16.0.0/12` (Docker bridge).

### Backup and restore

SQLite is a single file:

```bash
# Backup (safe while running — SQLite handles concurrent readers)
sqlite3 /data/homepage.db ".backup /data/backup-$(date +%Y%m%d).db"

# Or via docker compose:
docker compose exec app sqlite3 /data/homepage.db ".backup /tmp/bak.db"
docker compose cp app:/tmp/bak.db ./homepage-$(date +%Y%m%d).db
```

Restore: stop the server, replace the database file, start again.

### Local initialization before public deployment

1. Start the app locally with `APP_ENV=development` (or explicitly `APP_SETUP_ENABLED=true`).
2. Open `http://localhost:5173` in Vite dev mode or `http://localhost:8080` in built mode.
3. Use the setup UI once to create the admin account.
4. Stop the app and copy/restore `./data/homepage.db` to the server's `/data/homepage.db`.
5. Start production with `APP_ENV=production` and `APP_SETUP_ENABLED=false`, then sign in normally.

### Security checklist

- [ ] `APP_SESSION_SECRET` ≥ 32 random bytes, never committed to git
- [ ] `APP_ENV=production`
- [ ] `APP_BASE_URL` starts with `https://`
- [ ] `APP_COOKIE_SECURE=true` (default in production)
- [ ] Caddy obtains a real TLS certificate (Let's Encrypt)
- [ ] `.env` is NOT committed (already in `.gitignore`)
- [ ] Admin password is strong
- [ ] `APP_SETUP_ENABLED=false` on public servers
- [ ] Firewall: only ports 80/443 open; 8080 is internal
- [ ] Database file permissions: `chmod 600 /data/homepage.db`
- [ ] Regular backups configured

### JSON backup and restore

Export/import all your data (categories, tags, bookmarks, read-later items
and their tag associations) as a portable JSON file. **Passwords and sessions
are never exported.**

#### API

| Method | Path | Notes |
|--------|------|-------|
| GET | `/api/v1/backup/export` | Download JSON (RequireAuth) |
| POST | `/api/v1/backup/import` | Upload JSON (RequireAuth + CSRF) |

**Export** returns `application/json` with `Content-Disposition: attachment`.
Entity references (category, tags) use **names** not IDs, so the backup is
human-readable and portable across installations.

**Import modes:**
- `merge` (recommended) — add new items, update existing matched by URL/name.
  Does NOT delete existing data.
- `replace` — clears current user's bookmarks/readlater/categories/tags first,
  then inserts the backup. Atomic single transaction. Does NOT affect auth.

Limits: 10 MiB body, 10,000 entities/type, valid URLs, 2,000 char titles.
The response is an `ImportSummary` with per-entity created/updated/skipped
counts plus warnings/errors.

#### UI

The Settings page (`/settings`) provides:
- **Download backup** → saves `homepage-backup-YYYYMMDD.json`
- **Restore** → select JSON file, choose merge/replace. Replace shows a
  confirmation dialog.
- Import results displayed as a summary table.

### Smoke test

```bash
# 1. Health check (no auth)
curl -s http://localhost:8080/healthz | jq .
# → {"data":{"status":"ok","service":"homepage","time":"..."}}

# 2. For a fresh local dev database only: setup admin
#    Production should use a pre-initialized/restored database and skip this step.
curl -s -X POST http://localhost:8080/api/v1/auth/setup \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"strong-password"}'

# 3. Login → save cookies
curl -s -c cookies.txt -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"strong-password"}'

# 4. Get CSRF token (from cookies.txt header X-CSRF-Token value)

# 5. Create bookmark
curl -s -b cookies.txt -X POST http://localhost:8080/api/v1/bookmarks \
  -H 'Content-Type: application/json' -H 'X-CSRF-Token: <token>' \
  -d '{"url":"https://example.com","title":"Example"}'

# 6. Create read-later, trash, purge
ITEM=$(curl -s -b cookies.txt -X POST http://localhost:8080/api/v1/read-later \
  -H 'Content-Type: application/json' -H 'X-CSRF-Token: <token>' \
  -d '{"url":"https://example.com/article"}' | jq -r '.data.id')
curl -s -b cookies.txt -X DELETE \
  -H 'X-CSRF-Token: <token>' \
  "http://localhost:8080/api/v1/read-later/$ITEM"
curl -s -b cookies.txt -X DELETE \
  -H 'X-CSRF-Token: <token>' \
  "http://localhost:8080/api/v1/read-later/$ITEM/purge"
```

### Troubleshooting

| Symptom | Likely fix |
|---------|-----------|
| 502 Bad Gateway | App not healthy — `docker compose logs app` |
| TLS certificate error | DNS A/AAAA not pointing to server |
| Cookies not set | `APP_COOKIE_SECURE=true` but no HTTPS |
| Setup page loops | `APP_SESSION_SECRET` missing or < 32 bytes |
| DB locked | Multiple processes accessing SQLite file |
| Rate-limited | Too many failed logins; wait `APP_LOGIN_WINDOW` |

---

## Develop

```bash
make test     # go test ./...
make vet      # go vet ./...
make fmt      # gofmt -s -w .
make tidy     # go mod tidy
```

## Notes on the China Go proxy

If `go` commands time out, point the module proxy at a domestic mirror:

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

## Roadmap (phases)

1. ✅ Backend skeleton: config, logging, http server, request id,
   error model, SQLite + migrations, health.
2. ✅ Authentication & public-access security: setup/login/logout/me, session
   cookie, RequireAuth middleware, bcrypt, login rate-limit.
3. ✅ Bookmark core API: categories/tags/bookmarks CRUD, search & filter,
   soft-delete/restore, archive, open tracking — all behind `auth.RequireAuth`.
4. ✅ Read-later API: items CRUD, unread/reading/archive/trash flow,
   open tracking, favorite/priority, tags, search/filter/pagination.
5. 🔄 Import/export: browser-bookmarks HTML import done; JSON export/import,
   read-later-to-bookmark conversion, and metadata fetching still pending.
6. 🔄 Frontend: visual prototype + UI foundation done (this repo, `web/`);
   next is wiring it to the API, then PWA, themes, and deploy hardening
   (HTTPS reverse proxy, backups, CSRF/SSRF).
