# Tasks: mdserver

Standard verification for every task, in addition to the task's own checks:
`gofmt -l .` prints nothing, `go vet ./...` is clean, `go test -race ./...`
passes.

## Phase 1: Tree over HTTP

## Task 1: Scaffold module and `pathsafe`

**Description:** Create `go.mod` (`github.com/frrahat/markdown-server`,
`go 1.26`, `toolchain go1.27.3`), a minimal `cmd/mdserver` that parses flags and
the root argument, and `internal/pathsafe`, which turns a user-supplied
slash-separated relative path into a validated root-relative path and an
absolute filesystem path.

**Acceptance criteria:**
- [x] `pathsafe` rejects `..` segments, absolute paths, backslashes, NUL bytes
  and non-canonical forms, and returns `""` for the root. (Paths arrive
  percent-decoded, so `%2e%2e` is covered by the `..` check; the encoded form
  is tested over HTTP in Task 9.)
- [x] Resolving a path that goes through a symlink pointing outside the root
  returns an error
- [x] `go run ./cmd/mdserver -h` lists `-addr`, `-unsafe-html`, `-ext`,
  `-max-size`; a missing or non-directory root exits non-zero with a clear
  message

**Verification:**
- [x] `go test -race ./internal/pathsafe/...` (table-driven traversal cases)
- [x] `go build -o bin/mdserver ./cmd/mdserver`

**Dependencies:** None

**Files likely touched:** `go.mod`, `cmd/mdserver/main.go`,
`internal/pathsafe/pathsafe.go`, `internal/pathsafe/pathsafe_test.go`,
`.gitignore` (for `bin/`)

**Estimated scope:** M

## Task 2: Index scan: walk, skip rules, prune, sort

**Description:** `index.Scan(root, opts)` walks the tree once without following
symlinks and builds an immutable `Index` (`map[dir][]Entry`, generation,
indexedAt, file/dir counts). Skips `.git`, `node_modules` and dot-prefixed
files. Walks other dot-dirs. Matches extensions case-insensitively. Prunes
dirs with no markdown below them, bottom-up. Logs and skips unreadable dirs.

**Acceptance criteria:**
- [x] `Children(dir)` returns dirs first, then files, case-insensitively sorted
- [x] Empty and markdown-free dirs never appear; `.github/` with a `.md`
  appears; `.notes.md`, `.git/` and `node_modules/` never appear
- [x] A symlinked dir is not descended into; an unreadable subdir is skipped
  and the rest of the scan succeeds

**Verification:**
- [x] `go test -race ./internal/index/...` (fixtures in `t.TempDir()`)

**Dependencies:** None

**Files likely touched:** `internal/index/index.go`, `internal/index/scan.go`,
`internal/index/scan_test.go`

**Estimated scope:** S

## Task 3: Nested `.gitignore` support

**Description:** While walking, read each directory's `.gitignore` and add its
patterns scoped to that directory. Ignored dirs are not descended into. Store
the compiled filter on the `Index` with an `IsServable(relPath)` method for
`/raw/` to use later. (Library choice is pending — see plan Open Questions.)

**Acceptance criteria:**
- [x] A root `.gitignore` and a nested `.gitignore` both apply, scoped
  correctly (a nested pattern doesn't affect siblings of its dir)
- [x] Negation (`!keep.md`), dir-only (`build/`) and `**` patterns behave as
  in git
- [x] `IsServable` returns false for ignored paths, dotfiles, `.git/` and
  `node_modules/` contents

**Verification:**
- [x] `go test -race ./internal/index/...`

**Dependencies:** Task 2

**Files likely touched:** `internal/index/ignore.go`,
`internal/index/ignore_test.go`, `internal/index/scan.go`, `go.mod`, `go.sum`

**Estimated scope:** S

## Task 4: Scan performance benchmark

**Description:** A benchmark that generates about 50k files in 10k dirs
(markdown mixed with other files, some `.gitignore`s) and times `Scan`.
Optimize only if over budget.

**Acceptance criteria:**
- [x] `BenchmarkScan` exists and generates its fixture under `b.TempDir()`
- [x] Warm-cache scan is < 2 s on this machine, and the result is recorded in
  the commit message

**Verification:**
- [x] `go test -run=^$ -bench=BenchmarkScan -benchtime=3x ./internal/index/`

**Dependencies:** Task 3

**Files likely touched:** `internal/index/scan_bench_test.go`, possibly
`internal/index/scan.go`

**Estimated scope:** XS

## Task 5: `/api/tree` server, Host guard, startup wiring

**Description:** `index.Holder` holds the current index (atomic pointer).
`internal/server` provides the mux, the `/api/tree` handler, the loopback
`Host` guard, `no-store` on `/api/*`, and JSON error helpers. `main` scans
before listening, then serves.

**Acceptance criteria:**
- [x] `GET /api/tree?path=` returns the root's children in the spec's JSON
  shape; an unknown dir returns 404; an invalid path returns 400
- [x] A request with `Host: evil.com:8080` gets 403; `127.0.0.1`, `localhost`
  and `[::1]` with the port pass
- [x] `/api/*` responses carry `Cache-Control: no-store`

**Verification:**
- [x] `go test -race ./internal/server/...` (`httptest`)
- [x] Manual: `go run ./cmd/mdserver ~/some/repo` then
  `curl -s '127.0.0.1:8080/api/tree?path=' | jq`

**Dependencies:** Tasks 1, 3

**Files likely touched:** `internal/index/holder.go`,
`internal/server/server.go`, `internal/server/tree.go`,
`internal/server/server_test.go`, `cmd/mdserver/main.go`

**Estimated scope:** M

## Checkpoint A: After Tasks 1–5
- [x] All tests pass, vet and gofmt clean
- [x] Real-directory `curl` of `/api/tree` looks right
- [x] Benchmark result under 2 s
- [ ] Review with human before proceeding

## Phase 2: Rendering over HTTP

## Task 6: Base renderer: GFM, chroma classes, mermaid passthrough

**Description:** `render.New(opts)` configures goldmark with GFM, footnotes,
heading IDs and goldmark-highlighting using chroma CSS classes. Fenced
`mermaid` blocks become `<pre class="mermaid">` with escaped, unhighlighted
content. Generate the chroma CSS file used by the UI.

**Acceptance criteria:**
- [ ] Tables, task lists, strikethrough, autolinks and footnotes render
- [ ] Fenced Go code has chroma `class` attributes and no inline `style`
- [ ] A mermaid fence produces `<pre class="mermaid">` with its source
  escaped and unchanged

**Verification:**
- [ ] `go test -race ./internal/render/...` (golden files in
  `internal/render/testdata/`)

**Dependencies:** None (can run in parallel with Phase 1)

**Files likely touched:** `internal/render/render.go`,
`internal/render/mermaid.go`, `internal/render/render_test.go`,
`internal/render/testdata/*`, `web/chroma.css`

**Estimated scope:** M

## Task 7: Relative link rewriting

**Description:** An AST transformer that rewrites link and image destinations
relative to the document's directory: `.md`/`.markdown` links become
`#/<resolved>` (keeping the `#anchor`), other relative files become
`/raw/<resolved>`, and external links get `target="_blank" rel="noopener"`.
Absolute URLs and anchor-only links are left alone.

**Acceptance criteria:**
- [ ] The golden cases from the spec pass: relative md, md with anchor, image,
  external, anchor-only, `../` within the root
- [ ] A `../` that escapes the root is left unrewritten (or made inert),
  never turned into a `/raw/` path outside the root

**Verification:**
- [ ] `go test -race ./internal/render/...`

**Dependencies:** Task 6

**Files likely touched:** `internal/render/links.go`,
`internal/render/links_test.go`

**Estimated scope:** S

## Task 8: Sanitizer policy and `-unsafe-html`

**Description:** Apply a bluemonday policy to rendered output: common README
HTML plus everything the renderer emits. `-unsafe-html` skips it.

**Acceptance criteria:**
- [ ] `<details>`, `<summary>`, `<img>`, `<kbd>`, `<sup>` and `align` survive;
  `<script>`, `on*=` handlers, `javascript:` hrefs and `style` are stripped
- [ ] Heading ids, chroma classes, `class="mermaid"`, task-list checkboxes,
  footnote links and link `target`/`rel` survive
- [ ] With `UnsafeHTML: true`, raw HTML passes through unchanged

**Verification:**
- [ ] `go test -race ./internal/render/...`

**Dependencies:** Task 7

**Files likely touched:** `internal/render/sanitize.go`,
`internal/render/sanitize_test.go`, `internal/render/render.go`, `go.mod`,
`go.sum`

**Estimated scope:** S

## Task 9: `/api/doc` and `/raw/` endpoints

**Description:** `/api/doc` reads the file from disk on every request (only if
it's in the current index), enforces `-max-size` (413), and returns the
rendered HTML with modTime and size. `/raw/<path>` serves files that pass
`pathsafe` and `Index.IsServable`, with `nosniff` and
`Content-Security-Policy: sandbox`.

**Acceptance criteria:**
- [ ] `/raw/%2e%2e/x` and `/api/doc?path=%2e%2e%2Fx` are rejected (400)
- [ ] Editing a file on disk changes the next `/api/doc` response; a file
  that's not indexed returns 404; an oversized file returns 413
- [ ] `/raw/img/a.png` serves with a sniffed type and the security headers;
  dotfiles, ignored paths and symlink escapes return 404/400
- [ ] `-unsafe-html` and `-max-size` flags are wired through `main`

**Verification:**
- [ ] `go test -race ./internal/server/...`
- [ ] Manual: `curl -s '127.0.0.1:8080/api/doc?path=README.md' | jq -r .html`

**Dependencies:** Tasks 5, 8

**Files likely touched:** `internal/server/doc.go`, `internal/server/raw.go`,
`internal/server/doc_test.go`, `internal/server/raw_test.go`,
`cmd/mdserver/main.go`

**Estimated scope:** M

## Checkpoint B: After Tasks 6–9
- [ ] All tests pass, vet and gofmt clean
- [ ] `curl` of a real README returns sanitized, highlighted HTML with
  rewritten links
- [ ] Review with human before proceeding

## Phase 3: Browser UI

## Task 10: UI shell: lazy tree and doc view

**Description:** Embed `web/` with `go:embed` and serve it at `/` with ETag
caching. Two-pane layout. The tree fetches `/api/tree?path=` on load and a
dir's children on its first expand (cached client-side). Clicking a file
fetches `/api/doc` and inserts the server HTML. All other DOM is built with
`createElement`/`textContent`.

**Acceptance criteria:**
- [ ] First load makes exactly one tree request (`path=`); expanding a folder
  makes one request for that folder, and only on its first expand
- [ ] Clicking a file renders it with highlighting styles applied
- [ ] Static assets return an `ETag` and answer `If-None-Match` with 304

**Verification:**
- [ ] `go test -race ./internal/server/...` (static/ETag test)
- [ ] Manual in browser (DevTools network tab): request counts above

**Dependencies:** Task 9

**Files likely touched:** `web/embed.go`, `web/index.html`, `web/app.js`,
`web/style.css`, `internal/server/static.go`

**Estimated scope:** M

## Task 11: Hash routing, deep links, mermaid

**Description:** Route on `#/path/to/file.md`. Opening a file updates the hash;
`hashchange` handles back/forward and in-doc `.md` links. On load with a deep
link, fetch only the ancestor dirs, expand them, and open the file. Vendor
`mermaid.min.js` (pinned version, license noted) and run it with
`securityLevel: 'strict'` after each doc render.

**Acceptance criteria:**
- [ ] Reloading the page on a deep link reopens the file with its ancestors
  expanded, fetching only those ancestor dirs
- [ ] Back/forward move between viewed docs; relative `.md` links navigate in
  the viewer; images load from `/raw/`
- [ ] Mermaid blocks render as diagrams with no network requests outside the
  server

**Verification:**
- [ ] Manual in browser against a fixture tree with links, images and mermaid

**Dependencies:** Task 10

**Files likely touched:** `web/app.js`, `web/index.html`,
`web/vendor/mermaid.min.js`, `web/vendor/README.md`

**Estimated scope:** S

## Task 12: Refresh tree and Reload doc

**Description:** `Holder.Refresh()` rescans with single-flight coalescing and
swaps atomically; `POST /api/refresh` exposes it. UI **Refresh tree**: POST,
drop the client cache, re-fetch the root and every previously expanded dir that
still exists, in parallel. **Reload**: re-fetch the current doc and keep the
scroll position.

**Acceptance criteria:**
- [ ] Concurrent `Refresh()` calls trigger one scan; readers during a swap see
  either the old or the new index (`-race` test)
- [ ] A new file appears only after Refresh tree; expanded folders that still
  exist stay expanded; deleted ones vanish
- [ ] Reload shows edited content without a page reload and keeps the scroll
  position

**Verification:**
- [ ] `go test -race ./internal/index/... ./internal/server/...`
- [ ] Manual in browser: create/delete/edit files, then use the buttons

**Dependencies:** Task 11

**Files likely touched:** `internal/index/holder.go`,
`internal/index/holder_test.go`, `internal/server/refresh.go`,
`internal/server/refresh_test.go`, `web/app.js`

**Estimated scope:** M

## Task 13: Keyboard navigation and ARIA

**Description:** ARIA `tree`/`treeitem`/`group` roles with `aria-expanded`.
Up/Down move between visible items; Right expands or moves to the first child;
Left collapses or moves to the parent; Enter opens the file. Roving tabindex.

**Acceptance criteria:**
- [ ] The whole tree can be navigated and files opened with the keyboard only
- [ ] Roles and `aria-expanded` are correct in the DevTools accessibility pane

**Verification:**
- [ ] Manual in browser, keyboard only

**Dependencies:** Task 12

**Files likely touched:** `web/app.js`, `web/style.css`

**Estimated scope:** S

## Checkpoint C: Complete
- [ ] Every `SPEC.md` success criterion verified
- [ ] Frontend manual checklist done in a real browser: expand, refresh with
  expanded-state preservation, reload doc, deep link, back/forward, keyboard
  nav
- [ ] `go test -race -cover ./internal/...` shows ≥ 80% per package
- [ ] Single binary runs from an empty directory with no external files
