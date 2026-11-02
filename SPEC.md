# Spec: mdserver — local Markdown browser

## Objective

Run a local web server on a directory of markdown files. The browser shows:

- **Left pane:** a tree of every markdown file under the root. The tree loads
  lazily: only the root's direct children are sent at first, and a folder's
  children are fetched when you expand it.
- **Right pane:** the selected file, rendered as HTML.

The user is a developer reading notes or docs on their own machine. Success
means pointing it at a large repo, seeing the tree almost at once, expanding
folders without waiting, and reading rendered docs, diagrams included, without
leaving the browser.

### User stories

1. Run `mdserver ~/notes` and open `http://127.0.0.1:8080`. The top level of the
   tree appears.
2. Expand a folder. Its direct children appear: subfolders that contain markdown
   somewhere below them, plus `.md`/`.markdown` files. Folders with no markdown
   anywhere below them never appear.
3. Click a file. It renders in the right pane with GFM, highlighted code blocks
   and Mermaid diagrams. The URL changes to a shareable deep link, and
   browser back/forward work.
4. Click **Refresh tree** to rescan the filesystem. New files appear and
   deleted ones vanish. Folders that were expanded stay expanded if they still
   exist.
5. Click **Reload** while reading a file to re-render the latest content from
   disk. Tree state and scroll position are kept where possible.
6. A relative link to another `.md` file opens it in the viewer. A relative
   image (`![](img/a.png)`) displays. External links open in a new tab.

### Non-goals (v1)

- Editing files, auth, multiple users, network exposure
- Full-text content search, or filtering the tree by content
- Live push updates (file watching or websockets). Refresh is manual.
- Multiple root directories. Could be added later as top-level tree nodes.

## Design

### Assumptions

1. Localhost only. The server binds `127.0.0.1` by default, with no auth.
2. One root directory, given on the command line.
3. Discovery matches filenames only, with case-insensitive extensions `.md` and
   `.markdown`.
4. The walk skips `.git` and `node_modules` and honours `.gitignore` files,
   including nested ones. Other dot-prefixed **dirs** (e.g. `.github/`) are
   walked and appear if they contain markdown. Dot-prefixed **files** are
   skipped, so `.env`-style files never reach the index or `/raw/`.
5. Symlinks are not followed during the walk. That avoids cycles and escaping
   the root.
6. Raw HTML in markdown is passed through and then **sanitized** with an
   allowlist (bluemonday), so common README HTML (`<details>`, `<summary>`,
   `<img>`, `<kbd>`, `<sup>`, alignment attributes) renders, while scripts,
   event handlers and `javascript:` URLs are stripped. Unsanitized output would
   let a malicious README in a cloned repo run JS on our origin. The
   `-unsafe-html` flag skips the sanitizer.
7. Mermaid is vendored and embedded, so the binary grows by about 3 MB but works
   offline. No CDN.
8. Syntax highlighting happens server-side with chroma, so no JS is needed for
   it.

### Indexing ("index once, serve lazily")

- `Scan(root)` walks the tree once and builds an immutable `Index`. The index is
  `map[dirRelPath][]Entry`, with each slice sorted dirs-first and then
  case-insensitively by name. Only dirs that contain a markdown file somewhere
  in their subtree are kept, which is done by pruning bottom-up after the walk.
- The live index sits behind an `atomic.Pointer[Index]`. A refresh builds a new
  index and swaps it in, so readers never see a half-built tree. If several
  refreshes arrive at once, they share one scan.
- The server scans once at startup, before it starts listening.
- Per-directory reads that fail (permission denied, for example) are logged and
  skipped. They don't fail the whole scan.

### HTTP API

All paths are slash-separated and relative to the root. Every handler validates
the path: it must be clean, must not escape the root, and must not use `..`. It
must not be absolute or resolve through a symlink outside the root. Invalid
paths get `400`, and paths that are missing or not indexed get `404`.

| Method | Path | Purpose | Response |
|---|---|---|---|
| GET | `/` and static assets | UI shell (embedded) | HTML/JS/CSS |
| GET | `/api/tree?path=<dir>` | Direct children of `dir` (`""` = root) | `{"path","children":[{"name","path","type":"dir"\|"file"}],"generation","indexedAt"}` |
| POST | `/api/refresh` | Rescan and swap the index | `{"generation","indexedAt","files","dirs","durationMs"}` |
| GET | `/api/doc?path=<file>` | Read from disk **on every request**, render | `{"path","html","modTime","size"}` |
| GET | `/raw/<path>` | Serve a file under the root, for relative images | file bytes with a sniffed Content-Type |

- `/api/doc` accepts only files that are in the current index. Files over 5 MB
  get `413`.
- `/raw/` applies the same filter as the walk: it refuses dot-prefixed files,
  anything under `.git`/`node_modules`, and anything gitignored. It sends
  `X-Content-Type-Options: nosniff` and `Content-Security-Policy: sandbox`.
- **DNS-rebinding guard:** requests whose `Host` is not `127.0.0.1`, `localhost`
  or `[::1]` (with the port) get `403`.
- `/api/*` and `/raw/*` responses set `Cache-Control: no-store`, so Reload
  always gets fresh content. Embedded static assets are served with an ETag
  instead, so the ~3 MB mermaid bundle isn't re-downloaded on every page load.

### Rendering

- goldmark with the GFM extensions (tables, task lists, strikethrough,
  autolinks), footnotes, heading IDs, and chroma highlighting through
  `goldmark-highlighting`. Chroma emits CSS classes, not inline styles, so the
  sanitizer can drop `style` attributes without losing highlighting.
- The rendered HTML goes through the bluemonday policy (unless `-unsafe-html`).
  The policy must keep what the renderer itself emits: heading `id`s, chroma
  `class`es, `class="mermaid"`, task-list checkboxes, footnote links, and the
  rewritten link `target`/`rel`.
- An AST transformer rewrites relative link destinations against the document's
  directory:
  - `foo/bar.md#sec` becomes `#/foo/bar.md?anchor=sec`. The anchor is a query
    parameter, not a second `#`, because the sanitizer percent-encodes a second
    `#`, which would then look like a `#` in a file name.
  - other relative files become `/raw/<resolved>`
  - absolute URLs and `#anchor`-only links are left alone. External links get
    `target="_blank" rel="noopener"`.
- Relative URLs in **raw HTML** (`<img src="docs/logo.png">`, `<a href>`) are not
  rewritten by the server. The frontend resolves them against the document's
  directory before inserting the HTML, so it works with or without
  `-unsafe-html`.
- ```` ```mermaid ```` blocks are emitted as `<pre class="mermaid">` without
  highlighting. Client-side mermaid renders them after each load, with
  `securityLevel: 'strict'` so diagram labels can't inject HTML.

### Frontend

- Vanilla JS and CSS with no build step. The files are embedded with
  `go:embed`.
- The tree uses `<ul>` nodes. A node's children are fetched on its first
  expand and cached on the client until the next tree refresh.
- Routing uses the URL hash (`#/path/to/file.md`), so deep links survive a page
  reload, which expands the ancestors of the linked file.
- **Refresh tree:** POST `/api/refresh`, drop the client cache, then re-fetch
  root and re-expand every previously expanded dir that still exists, one
  depth level at a time (in parallel within a level). A dir is only fetched if
  it still appears in its parent's new listing, so deleted dirs cause no 404s.
- **Reload doc:** re-fetch `/api/doc` for the current file and keep the
  scroll position.
- Keyboard: arrow keys move through the tree, and Enter opens. Uses ARIA
  `tree`/`treeitem` roles.

## Tech Stack

- Module path `github.com/frrahat/markdown-server` (private repo)
- Go 1.26, stdlib `net/http` (1.22+ pattern routing), `log/slog`
- `github.com/yuin/goldmark`, `github.com/yuin/goldmark-highlighting/v2`
  (chroma)
- `github.com/microcosm-cc/bluemonday` (HTML sanitizer)
- A gitignore matcher library, chosen in the plan, that supports nested
  `.gitignore`
- Mermaid (vendored minified JS)

## Commands

```
Run:    go run ./cmd/mdserver -addr 127.0.0.1:8080 ~/notes
Build:  go build -o bin/mdserver ./cmd/mdserver
Test:   go test -race ./...
Vet:    go vet ./...
Format: gofmt -l . (must print nothing); gofmt -w . to fix
```

CLI flags: `-addr` (default `127.0.0.1:8080`), `-unsafe-html` (default off),
`-ext` (default `.md,.markdown`), `-max-size` (default `5MB`). The single
positional argument is the root directory.

## Project Structure

```
cmd/mdserver/main.go     → flag parsing, startup scan, server start
internal/index/          → walk, ignore rules, pruning, Index type, refresh coalescing
internal/render/         → goldmark setup, link-rewrite transformer, mermaid passthrough
internal/server/         → HTTP handlers, path validation, Host guard
internal/pathsafe/       → resolve and validate user-supplied relative paths (shared)
web/                     → index.html, app.js, style.css, vendor/mermaid.min.js, embed.go
testdata/                → fixture markdown trees used by tests
SPEC.md                  → this document
tasks/                   → plan.md, todo.md
```

## Code Style

Standard `gofmt`. Use small packages with exported types only where another
package needs them. Return errors and don't panic. Wrap errors with `%w` and
add context. Use descriptive names. Comments explain *why*, not *what*.

```go
// Children returns the direct children of dirPath. Directories with no
// markdown anywhere beneath them were pruned at scan time, so every returned
// dir is guaranteed to expand to something.
func (idx *Index) Children(dirPath string) ([]Entry, bool) {
	entries, found := idx.childrenByDir[dirPath]
	return entries, found
}

func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	dirPath, err := pathsafe.Clean(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	idx := s.indexHolder.Current()
	entries, found := idx.Children(dirPath)
	if !found {
		writeError(w, http.StatusNotFound, fmt.Errorf("directory %q not indexed", dirPath))
		return
	}
	writeJSON(w, treeResponse{Path: dirPath, Children: entries, Generation: idx.Generation})
}
```

JS uses ES modules and `const`/`let`, with no frameworks. DOM is built through
`document.createElement` or `textContent`. Never use `innerHTML` with
unescaped data. The server-rendered doc HTML is the one deliberate exception.

## Testing Strategy

- **Framework:** stdlib `testing`, table-driven, with `-race` on.
- **`internal/index`:** fixture trees built in `t.TempDir()`. Cases: empty-dir
  pruning, nested `.gitignore`, dot-dirs walked but dotfiles and `.git`
  skipped, `node_modules`, symlink not
  followed, sort order, case-insensitive extension, an unreadable subdir
  skipped, and concurrent refresh coalescing with readers during a swap.
- **`internal/pathsafe`:** traversal cases (`..`, `%2e%2e`, absolute paths,
  backslashes, NUL bytes, symlink escapes).
- **`internal/render`:** golden-output tests for link rewriting (relative md,
  md with anchor, image, external, anchor-only, `../` within the root, `../`
  escaping the root), mermaid passthrough. Sanitizer cases: `<details>`/`<img>`
  kept; `<script>`, `onerror=`, `javascript:` hrefs and `style` stripped;
  heading ids, chroma classes and `class="mermaid"` survive; `-unsafe-html`
  bypasses the sanitizer.
- **`internal/server`:** `httptest` for each endpoint: status codes, JSON
  shape, Host guard, `no-store` header, 413 on oversize, doc content changes
  after the file is edited (no caching), tree reflects a new file only after
  refresh.
- **Frontend:** a manual checklist in the plan (expand, refresh with
  expanded-state preservation, reload doc, deep link, back/forward, keyboard
  nav), checked in a real browser.
- **Coverage:** ≥ 80% statements in `internal/...`. No coverage target for
  `cmd/` or `web/`.

## Boundaries

- **Always:** run `gofmt`, `go vet` and `go test -race ./...` before each
  commit. Validate every user-supplied path through `pathsafe`. Keep the
  default bind on loopback.
- **Ask first:** adding any Go dependency beyond goldmark,
  goldmark-highlighting, bluemonday and the gitignore matcher. Loosening the
  sanitizer policy beyond what the renderer emits plus common README HTML. Changing the default bind
  address or the raw-HTML default. Adding a JS build step or framework.
  Changing the API shape after it's implemented.
- **Never:** follow symlinks out of the root. Serve dotfiles or paths outside
  the root. Load JS from a CDN. Write to the served directory. Delete or skip
  failing tests to get green.

## Success Criteria

1. `go build` produces one binary that runs with no external files, mermaid
   included.
2. On a fixture tree of about 50k files and about 10k dirs (warm cache, SSD),
   the startup scan takes < 2 s, and `/api/tree` p95 is < 20 ms locally.
   Each response holds only one directory's children.
3. A first page load without a deep link fetches only `/api/tree?path=` and
   makes no other tree calls until the user expands a folder. With a deep link,
   it fetches only the linked file's ancestor dirs.
4. No folder in the tree is empty when expanded.
5. After a file is created or deleted on disk, the tree is unchanged until
   **Refresh tree** is clicked, and correct afterwards. Expanded folders that
   still exist stay expanded.
6. After a file is edited on disk, **Reload** shows the new content without a
   page reload.
7. Relative `.md` links navigate inside the viewer, relative images render,
   fenced code is highlighted, and Mermaid blocks render as diagrams.
8. Every path-traversal and Host-guard test passes. Requests with a
   non-loopback `Host` header get `403`.
9. `go vet` is clean, `gofmt -l` is empty, and coverage in `internal/...` is
   ≥ 80%.

## Open Questions

None blocking. The gitignore matcher library is chosen during planning.
