package index

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Holder owns the current Index. Readers call Current and get a consistent
// snapshot without locking; Refresh swaps in a new one atomically.
type Holder struct {
	root           string
	opts           Options
	current        atomic.Pointer[Index]
	lastGeneration atomic.Uint64

	// scan is Scan, replaceable in tests to observe coalescing.
	scan func(root string, opts Options) (*Index, error)

	refreshMu       sync.Mutex
	inflightRefresh *refreshCall
}

type refreshCall struct {
	done chan struct{}
	idx  *Index
	err  error
}

// NewHolder runs the initial scan so the server never serves without an
// index.
func NewHolder(root string, opts Options) (*Holder, error) {
	holder := &Holder{root: root, opts: opts, scan: Scan}
	if _, err := holder.scanAndPublish(); err != nil {
		return nil, err
	}
	return holder, nil
}

func (h *Holder) Current() *Index {
	return h.current.Load()
}

// Refresh rescans the root and publishes the result. Calls that arrive while
// a scan is running wait for that scan and share its result instead of
// starting another, so a burst of refresh clicks costs one walk. On error the
// previous index stays in place.
func (h *Holder) Refresh() (*Index, error) {
	h.refreshMu.Lock()
	if call := h.inflightRefresh; call != nil {
		h.refreshMu.Unlock()
		<-call.done
		return call.idx, call.err
	}
	call := &refreshCall{done: make(chan struct{})}
	h.inflightRefresh = call
	h.refreshMu.Unlock()

	call.idx, call.err = h.scanAndPublish()

	h.refreshMu.Lock()
	h.inflightRefresh = nil
	h.refreshMu.Unlock()
	close(call.done)
	return call.idx, call.err
}

func (h *Holder) scanAndPublish() (*Index, error) {
	idx, err := h.scan(h.root, h.opts)
	if err != nil {
		return nil, fmt.Errorf("index %s: %w", h.root, err)
	}
	// Set before Store: the Index must not change once readers can see it.
	idx.Generation = h.lastGeneration.Add(1)
	h.current.Store(idx)
	return idx, nil
}
