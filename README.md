# mdserver

A local web server for reading a directory of Markdown files in the browser.
The left pane shows a tree of every Markdown file under the directory; the
right pane renders the selected file, with GitHub-flavoured Markdown,
syntax-highlighted code and Mermaid diagrams.

It ships as a single binary with everything embedded, works offline, and
binds to localhost only.

## Quick start

```sh
make run ROOT=~/notes
```

Then open <http://127.0.0.1:8642>.

Go 1.22 or newer is enough to start: `go.mod` pins the toolchain
(go1.27.3), and the `go` command downloads it automatically on first use.

## Make targets

| Target | What it does |
|---|---|
| `make run ROOT=<dir>` | Serve `<dir>` (default `.`). Optional `ADDR=127.0.0.1:9090`, `FLAGS=-unsafe-html` |
| `make build` | Build the binary at `bin/mdserver` |
| `make test` | Run all tests with the race detector |
| `make check` | `gofmt` check, `go vet`, and tests: run before committing |
| `make clean` | Remove `bin/` |

The built binary takes the same options directly:

```sh
bin/mdserver [flags] <root-dir>
```

| Flag | Default | Meaning |
|---|---|---|
| `-addr` | `127.0.0.1:8642` | Listen address |
| `-ext` | `.md,.markdown` | Comma-separated Markdown extensions (case-insensitive) |
| `-max-size` | `5MB` | Largest file that will be rendered (`512KB`, `5MB`, or bytes) |
| `-unsafe-html` | off | Render raw HTML in documents without sanitizing it |

## Features

- **Fast on large trees.** The directory is scanned once at startup; folders are
  then served from memory and fetched one level at a time as you expand them.
  A 50k-file tree scans in about 0.3 s.
- **Only what matters is shown.** Folders with no Markdown anywhere beneath
  them never appear. `.git`, `node_modules`, dotfiles and anything matched by
  `.gitignore` (including nested ones) are skipped. Other dot-folders such as
  `.github/` are shown.
- **Rendering.** Tables, task lists, footnotes, strikethrough, autolinks,
  heading anchors, highlighted code, and Mermaid diagrams.
- **Links work like on GitHub.** Relative links to other `.md` files open in
  the viewer, relative images load, `/path` links resolve from the root, and
  external links open in a new tab.
- **Shareable URLs.** Each file has a deep link (`#/docs/guide.md`), and
  browser back/forward work.
- **Manual refresh, no file watching.**
  - **Refresh tree** rescans the directory for added or removed files, and
    keeps open folders open.
  - **Reload** re-reads the current file from disk, and keeps your scroll
    position.
- **Keyboard navigation** in the tree: arrow keys, Home/End, Enter.

## Security model

mdserver is meant for reading files on your own machine, possibly from repos
you cloned but didn't write.

- Binds to `127.0.0.1` by default and has no authentication.
- Requests with a `Host` header other than `localhost`, `127.0.0.1` or `[::1]`
  are rejected, which blocks DNS-rebinding attacks from other websites.
- Raw HTML in Markdown is sanitized (common README tags like `<details>`,
  `<img>` and `<kbd>` are kept; scripts, event handlers and `javascript:`
  links are removed). The page also sends a Content-Security-Policy that
  blocks inline and third-party scripts. External `https` images are
  allowed so README badges render.
- Paths are validated on every request; symlinks are never followed during the
  scan, and nothing outside the root, no dotfile, and no gitignored file is
  ever served.

`-unsafe-html` turns the sanitizer off. Only use it on content you trust.

## HTTP API

| Method | Path | Returns |
|---|---|---|
| `GET` | `/api/tree?path=<dir>` | Direct children of a folder (`""` is the root) |
| `GET` | `/api/doc?path=<file>` | Rendered HTML for a file, read fresh from disk |
| `POST` | `/api/refresh` | Rescans the directory; returns file/dir counts and duration |
| `GET` | `/raw/<path>` | A file under the root, such as an image a document references |

## Project layout

```
cmd/mdserver/       CLI flags, startup scan, HTTP server
internal/index/     directory scan, .gitignore rules, in-memory index, refresh
internal/render/    Markdown → HTML, link rewriting, sanitizer, mermaid passthrough
internal/server/    HTTP handlers, Host guard, embedded static files
internal/pathsafe/  validation of user-supplied paths
web/                browser UI (vanilla JS, no build step) and vendored mermaid
SPEC.md             the specification
tasks/              implementation plan and task list
```

## Development

```sh
make check                                   # before each commit
go test ./internal/render/ -update           # regenerate golden HTML after an intended change
go test -run='^$' -bench=BenchmarkScan ./internal/index/   # scan performance
```

The frontend is plain ES modules embedded with `go:embed`; edit the files in
`web/` and rebuild. Mermaid is vendored in `web/vendor/`; see
`web/vendor/README.md` for its version and how to update it.
