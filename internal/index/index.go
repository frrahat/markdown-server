// Package index scans a root directory once and answers "what are the
// children of this directory" from memory.
package index

import "time"

type EntryType string

const (
	EntryDir  EntryType = "dir"
	EntryFile EntryType = "file"
)

// Entry is one child shown in the tree. Path is slash-separated and relative
// to the root, in the same canonical form pathsafe.Clean accepts.
type Entry struct {
	Name string    `json:"name"`
	Path string    `json:"path"`
	Type EntryType `json:"type"`
}

// Index is an immutable snapshot of the markdown tree. It is never modified
// after it is published, so it can be read concurrently without locking.
type Index struct {
	Generation uint64
	IndexedAt  time.Time
	FileCount  int
	// DirCount excludes the root.
	DirCount int

	childrenByDir map[string][]Entry
}

// Children returns the direct children of dirPath ("" is the root).
// Directories with no markdown anywhere beneath them were pruned at scan
// time, so every returned dir is guaranteed to expand to something. found is
// false if dirPath is not an indexed directory.
func (idx *Index) Children(dirPath string) (entries []Entry, found bool) {
	entries, found = idx.childrenByDir[dirPath]
	return entries, found
}

// HasFile reports whether filePath is an indexed markdown file.
func (idx *Index) HasFile(filePath string) bool {
	parentDir, fileName := splitParent(filePath)
	for _, entry := range idx.childrenByDir[parentDir] {
		if entry.Type == EntryFile && entry.Name == fileName {
			return true
		}
	}
	return false
}
