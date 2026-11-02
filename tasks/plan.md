# Implementation Plan: mdserver

## Overview

Build the local Markdown browser described in `SPEC.md`: a single Go binary
that scans a root directory once, serves the markdown tree lazily over a JSON
API, renders documents server-side (GFM, chroma, sanitized HTML, mermaid
passthrough), and ships an embedded vanilla-JS UI. Tasks are recorded in
`tasks/todo.md`.

## Architecture Decisions

- **Toolchain:** `go.mod` declares `go 1.26` plus `toolchain go1.27.3`. The
  local Go (1.21.4) auto-downloads the newer toolchain via `GOTOOLCHAIN=auto`,
  so nobody has to upgrade by hand. Go 1.22+ is a hard floor because the
  server uses method/pattern routing.
- **Gitignore matcher:** `github.com/go-git/go-git/v5/plumbing/format/gitignore`
  (v5.19.3). Maintained, and `ParsePattern(line, domain)` lets the walker
  attach each nested `.gitignore` to its directory as it descends, which is
  exactly the nested semantics the spec needs. Cost: it pulls the go-git module
  into `go.sum`, although only the small `gitignore` package (plus go-billy) is
  compiled in. Alternatives: `sabhiram/go-gitignore` (last commit 2021) and
  `denormal/go-gitignore` (2018) are unmaintained; a hand-rolled matcher means
  owning `**`, negation and anchoring edge cases. See Open Questions.
- **Library versions (checked 2026-11-02):** goldmark v1.8.7,
  goldmark-highlighting/v2 (2023 pseudo-version, latest available) on chroma
  v2.27.0, bluemonday v1.0.27.
- **Ignore filter is a shared value.** The scan builds the ignore rules once
  per generation and stores them on the `Index`, so `/raw/` applies the exact
  same filter as the walk without re-reading `.gitignore` files per request.
- **Index holder owns refresh coalescing.** `index.Holder` wraps the
  `atomic.Pointer[Index]` and a single-flight refresh. Handlers only ever call
  `Current()` or `Refresh()`.
- **Frontend in two passes.** First a working tree + doc view (lazy expand,
  hash routing), then refresh/reload and keyboard/ARIA. Each pass is usable on
  its own.
- **Perf is measured early.** A generated 50k-file fixture benchmark runs right
  after the walker exists, so a slow ignore matcher shows up before the server
  is built on top of it.

## Dependency Graph

```
pathsafe ──┬──────────────► server ──► cmd/mdserver
index ─────┤                  ▲
render ────┘ (sanitize, links)│
web (embedded UI) ────────────┘
```

`pathsafe`, `index` and `render` are independent of each other and could be
built in parallel. `server` needs all three. The UI needs the API.

## Task List

Full details (acceptance, verification, files) live in `tasks/todo.md`.

### Phase 1: Tree over HTTP
- [x] 1. Scaffold module and `pathsafe`
- [ ] 2. Index scan: walk, skip rules, prune, sort
- [ ] 3. Nested `.gitignore` support
- [ ] 4. Scan performance benchmark
- [ ] 5. `/api/tree` server, Host guard, startup wiring

### Checkpoint A
`curl` against a real directory returns the root's children; tests, vet,
gofmt clean; scan benchmark under budget.

### Phase 2: Rendering over HTTP
- [ ] 6. Base renderer: GFM, chroma classes, mermaid passthrough
- [ ] 7. Relative link rewriting
- [ ] 8. Sanitizer policy and `-unsafe-html`
- [ ] 9. `/api/doc` and `/raw/` endpoints

### Checkpoint B
`curl /api/doc` returns sanitized, highlighted HTML with rewritten links;
`/raw/` serves images and refuses dotfiles/ignored paths.

### Phase 3: Browser UI
- [ ] 10. UI shell: lazy tree and doc view
- [ ] 11. Hash routing, deep links, mermaid
- [ ] 12. Refresh tree and Reload doc
- [ ] 13. Keyboard navigation and ARIA

### Checkpoint C (complete)
All spec success criteria pass, frontend manual checklist checked in a real
browser, `internal/...` coverage ≥ 80%.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| Ignore matching makes the 50k-file scan exceed 2 s | Med | Task 4 benchmarks right after the walker exists; skip ignored dirs without descending; parallelize the walk only if needed |
| Sanitizer strips something the renderer emits (ids, chroma classes, mermaid, checkboxes) | Med | Policy built from an explicit list in Task 8, with a golden test asserting each survives |
| goldmark-highlighting/v2 is a 2023 pseudo-version | Low | It is a thin adapter over chroma; if it breaks, call chroma directly from a small renderer hook |
| Symlink escapes through `/raw/` or `/api/doc` | High | `pathsafe` resolves with `filepath.EvalSymlinks` and re-checks containment; dedicated tests in Task 1 and Task 9 |
| Mermaid bundle size (~3 MB) bloats binary and page load | Low | Accepted in spec; ETag caching on static assets (Task 10) |
| Concurrent refresh vs. readers | Med | Atomic pointer swap + single-flight; `-race` test with readers during swap (Task 12) |

## Open Questions

1. **Gitignore library:** OK to add `github.com/go-git/go-git/v5` (using only
   its `plumbing/format/gitignore` package)? The alternative is a hand-rolled
   matcher of roughly 150 lines plus tests, which avoids the dependency.
