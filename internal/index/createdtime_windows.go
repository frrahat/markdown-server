package index

import (
	"io/fs"
	"syscall"
	"time"
)

func fileCreatedAt(info fs.FileInfo) time.Time {
	if attributes, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, attributes.CreationTime.Nanoseconds())
	}
	return info.ModTime()
}
