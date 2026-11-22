//go:build darwin || freebsd || netbsd

package index

import (
	"io/fs"
	"syscall"
	"time"
)

// fileCreatedAt returns the file's birth time, which these platforms record
// in every stat result.
func fileCreatedAt(info fs.FileInfo) time.Time {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(stat.Birthtimespec.Unix())
	}
	return info.ModTime()
}
