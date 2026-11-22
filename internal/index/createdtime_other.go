//go:build !(darwin || freebsd || netbsd || windows)

package index

import (
	"io/fs"
	"time"
)

// fileCreatedAt returns the modification time, so an edited file counts as
// recently created. The stat result has no birth time on these platforms;
// Linux exposes one only through statx.
func fileCreatedAt(info fs.FileInfo) time.Time {
	return info.ModTime()
}
