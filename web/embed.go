// Package web holds the browser UI, embedded into the binary so it runs with
// no external files.
package web

import "embed"

//go:embed index.html *.js style.css vendor/mermaid.min.js
var Files embed.FS
