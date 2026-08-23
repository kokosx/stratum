# AGENTS.md

## Project

StratumCMS is a modern self-hosted CMS written in Go.

Core idea:

> WordPress familiarity. Modern architecture. One binary.

Current goal:

```text
stratum serve
→ setup
→ login
→ create Page
→ add blocks
→ publish
→ public URL
```

Do not build the entire future CMS upfront.

## Priorities

Prefer:

1. simplicity
2. correctness
3. maintainability
4. security
5. performance
6. extensibility

Avoid overengineering and speculative abstractions.

## Stack

- Go
- Turso / SQLite
- `database/sql`
- `sqlc`
- server-rendered HTML
- minimal JS
- Datastar / SSE where useful
- single binary deployment

## Architecture

Prefer feature-oriented packages:

```text
internal/
├── app/
├── content/
├── document/
├── blocks/
├── media/
├── routing/
├── rendering/
├── auth/
├── web/
└── storage/
```

Avoid unnecessary `domain/application/infrastructure/shared` layering.

The database is the source of truth. Caches and registries must be rebuildable.

## Content

Pages and Posts are both `Entry` records differentiated by `content_type`.

Do not create separate Page/Post storage models.

Content uses revisions. Editing a published entry must not modify the public version until publish.

## Documents and Blocks

Content is stored as a structured document tree, not HTML.

Nodes have stable IDs and contain:

```text
block type
block version
props
settings
children
```

Block definitions are stored in the database and may be cached in memory.

Custom blocks must be installable without recompiling Stratum.

Keep block definitions separate from block instances inside revisions.

Block schemas must be versioned.

## Rendering

Use one rendering pipeline:

```text
Entry
→ Revision
→ Document
→ Blocks
→ Theme
→ HTML
```

Preview and public rendering should use the same renderer.

Themes control markup, layouts and CSS. Content must stay presentation-independent.

## Database

Use explicit SQL and `sqlc`.

Use constraints such as:

```text
FOREIGN KEY
UNIQUE
CHECK
NOT NULL
```

Do not manually edit generated `sqlc` files.

Current core tables:

```text
content_types
entries
entry_revisions
block_definitions
routes
site_settings
```

Do not add unused future tables.

## Site settings

Core site settings use one typed singleton row with columns such as:

```text
site_title
homepage_mode
homepage_entry_id
posts_page_entry_id
language
timezone
active_theme
```

Do not copy WordPress-style generic `wp_options` for core settings.

## Scope

Needed now:

```text
setup
auth
Page/Post
revisions
blocks
media
publish
routing
rendering
theme
```

Later:

```text
e-commerce
multisite
marketplace
GraphQL
CRDT
WASM extensions
```

Do not implement later features unless explicitly requested.

## Rules for changes

Be especially careful with:

```text
data model
document format
stable IDs
block schemas
block versioning
revisions
routing
public extension APIs
```

Before adding an abstraction, package or table, ask whether the current vertical slice actually needs it.

Prefer boring, explicit, easy-to-debug code.

## Admin UI interaction pattern (Datastar)

The admin is HTML-first and server-driven. Business logic, validation, DB
writes, revisions and routing always happen on the server. Datastar/SSE is only
the transport for interactive fragments — never the source of truth. When adding
or changing any admin interaction, follow this pattern:

```text
action (button/link with data-on:click="@post(...)" or data-on:submit)
→ server handler validates + writes + renders
→ Datastar merges a fragment (datastar-patch-elements) into the DOM
→ reuse #admin-toast-region for success/error toasts
```

Rules:

- Reusable helper lives in `internal/web/admin/datastar.go`:
  `writeSSE`, `patchElementsEvent`, `toastEvent`. Use these; do not hand-roll
  SSE or return plain `text/html` for fragment updates (Datastar needs
  `text/event-stream`).
- Every interactive button that should avoid a full reload must use
  `data-on:click__prevent="@post('/path', {contentType: 'form'})"` (the
  `__prevent` modifier stops the native form submit). The server detects the
  request via `isDatastarRequest(r)` (the `Datastar-Request: true` header) and
  answers with SSE fragment patches instead of a redirect/render.
- Keep progressive enhancement: classic `<form method="post" action="...">`
  must still work with JS disabled. Critical forms (create/delete) keep the
  full-page POST fallback; the Datastar branch only replaces fragments.
- Do NOT move business logic into JS. Do NOT turn the block editor into a
  server round-trip on every keypress: typing, drag/drop and selection stay
  client-side; Datastar is used for persistence and server-rendered fragments.
- Do NOT add React/Vue or a client-side router. One binary, server-rendered.
- Inline validation errors appear in a stable fragment element (e.g.
  `#editor-error`); the shared toast area (`#admin-toast-region`, populated by
  `toastEvent` server-side or `window.stratumToast` client-side) shows
  success/error without page reload.
- Status indicators (e.g. `Unsaved` → `Saved`, `Draft` → `Published` + public
  URL) are server-rendered fragments, not client guesses.
- If you run a stratum server, kill it at the end of your task.

## Architecture Invariants (must hold)

1. Code outside a concrete block definition/implementation MUST NOT branch on a concrete block name (`core/posts`, `core/content-slot`, etc.). Generic layers use block capabilities / editor contexts instead.

2. Generic content code MUST NOT branch on `contentType == "post"` if the difference can be derived from `ContentTypeDefinition`.

3. HTTP handlers MUST NOT know that a concrete dynamic block needs data (no `latestCollections()` / `preparePostsBlock()` in handlers). Data is fetched via `ContentReader` / `EntryQuery` inside the rendering scope.

4. Cache is never source of truth; it is rebuildable from DB.

5. Preview uses the exact same pipeline as public render (`Entry → Revision → SDT → Blocks → Theme → HTML`).

6. Future WASM plugins MUST NOT receive a direct `*sql.DB` handle; they use host capabilities (`content.read`, `media.read`, etc.).

7. A dynamic block is a normal block; "dynamic" is only runtime behaviour, not a separate SDT node type.

8. Composite / pattern / component features MUST NOT introduce a second document system; patterns expand to normal SDT, components resolve to normal SDT in the current `RenderContext`.

Primitives expected after refactor:

```
Entry, Revision, ContentTypeDefinition, Route, SDT, BlockDefinition,
RenderContext, EntryQuery, Collection, LayoutTemplate, Theme, Event, Cache
```

Most CMS features should be composition of these, not new special-case renderers/handlers.

## Routes & Content Types

- `routes.content_type_id` is nullable and set for `route_type='archive'` (e.g. `post` for `/blog`). Single entry routes use `entry_id` → content type via join.
- `routing` package (`internal/routing`) owns `EntryPath`, `PostsArchivePath`, `ValidatePostsBasePath`, route invariants, and `SyncPostsPageSlugChanged`. `seo` only receives already-resolved paths.
- `content` package owns `ContentTypeDefinition` and `EntryQuery`; handlers and renderers use it instead of hard-coded `post`/`page` switches.
- `Collection` (`core/collection`) is the generic replacement for `core/posts`. It fetches via `EntryQuery` + `ContentReader` and renders children with a scoped `Entry` context (`lazy RenderChildren`). `core/posts` remains as deprecated historical renderer for old docs but is hidden from new inserts.

## Rendering

- `RenderContext` has generic scopes: `Site`, `Entry`, `Route`, `Mode`, `ContentReader`, `QueryCache`. Blog-specific fields (`Archive`, `Collections`, `ArchiveURL`) remain for backward compat but new code uses `Route` + `ContentReader`.
- Collection uses request-local `QueryCache` so duplicate identical queries in one request hit the DB once (no global cache).

## Layout Templates

- `Compose` uses `document.Clone` (no JSON Marshal/Unmarshal pool). Composed SDT must pass `BlockRegistry.ValidateDocument(composed)`, not just `document.Validate`.
- `internal/layouts` should be accessed via a `LayoutTemplateReader` / `LayoutTemplateService` boundary, not directly via `*sqlc.Queries` in generic code.

## Caching & Events

- `pagecache` is whole-page HTML + gzip + ETag. `ArtifactCache` (generic) replaces duplicated `SitemapCache`/`RobotsCache`/`FeedCache` implementations; runtime keeps three named instances (`Sitemap`, `Robots`, `Feed`) for readability.
- `runtimehub.Dispatcher` is a tiny synchronous in-process event dispatcher (`EntryPublished`, `ThemeUpdated`, ...). `RuntimeHub` subscribes; no broker.
