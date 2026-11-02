package index

import (
	"fmt"
	"sync/atomic"
)

// Holder owns the current Index. Readers call Current and get a consistent
// snapshot without locking.
type Holder struct {
	root           string
	opts           Options
	current        atomic.Pointer[Index]
	lastGeneration atomic.Uint64
}

// NewHolder runs the initial scan so the server never serves without an
// index.
func NewHolder(root string, opts Options) (*Holder, error) {
	holder := &Holder{root: root, opts: opts}
	if _, err := holder.scanAndPublish(); err != nil {
		return nil, err
	}
	return holder, nil
}

func (h *Holder) Current() *Index {
	return h.current.Load()
}

func (h *Holder) scanAndPublish() (*Index, error) {
	idx, err := Scan(h.root, h.opts)
	if err != nil {
		return nil, fmt.Errorf("index %s: %w", h.root, err)
	}
	// Set before Store: the Index must not change once readers can see it.
	idx.Generation = h.lastGeneration.Add(1)
	h.current.Store(idx)
	return idx, nil
}
