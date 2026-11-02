package index

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHolderRefreshPicksUpChanges(t *testing.T) {
	root := buildTree(t, []string{"a.md", "old/b.md"})
	holder := newTestHolder(t, root)
	first := holder.Current()

	writeTestFile(t, root, "new/c.md", "# c\n")
	if err := os.RemoveAll(filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	// Nothing changes until Refresh.
	if holder.Current() != first {
		t.Fatal("index changed without Refresh")
	}

	refreshed, err := holder.Refresh()
	if err != nil {
		t.Fatal(err)
	}
	if holder.Current() != refreshed || refreshed.Generation != first.Generation+1 {
		t.Fatalf("generation %d -> %d", first.Generation, refreshed.Generation)
	}
	assertChildren(t, refreshed, "", []string{"dir:new", "file:a.md"})
	assertChildren(t, first, "", []string{"dir:old", "file:a.md"})
}

func TestHolderRefreshFailureKeepsPreviousIndex(t *testing.T) {
	holder := newTestHolder(t, buildTree(t, []string{"a.md"}))
	previous := holder.Current()
	holder.scan = func(string, Options) (*Index, error) { return nil, errors.New("disk gone") }
	if _, err := holder.Refresh(); err == nil {
		t.Fatal("Refresh succeeded, want error")
	}
	if holder.Current() != previous {
		t.Fatal("failed refresh replaced the index")
	}
}

func TestHolderRefreshCoalescesConcurrentCalls(t *testing.T) {
	root := buildTree(t, []string{"a.md"})
	holder := newTestHolder(t, root)

	var scanCount atomic.Int32
	scanStarted := make(chan struct{})
	releaseScan := make(chan struct{})
	holder.scan = func(root string, opts Options) (*Index, error) {
		if scanCount.Add(1) == 1 {
			close(scanStarted)
		}
		<-releaseScan
		return Scan(root, opts)
	}

	const callers = 8
	results := make(chan *Index, callers)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		idx, _ := holder.Refresh()
		results <- idx
	}()
	<-scanStarted
	for range callers - 1 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			idx, _ := holder.Refresh()
			results <- idx
		}()
	}

	// Readers keep working during the scan and always see a complete index.
	stopReaders := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stopReaders:
					return
				default:
					if _, found := holder.Current().Children(""); !found {
						t.Error("reader saw an index without a root")
						return
					}
				}
			}
		}()
	}

	// Give the joining callers time to block on the in-flight scan.
	time.Sleep(50 * time.Millisecond)
	close(releaseScan)
	wg.Wait()
	close(stopReaders)
	readers.Wait()
	close(results)

	if got := scanCount.Load(); got != 1 {
		t.Errorf("%d concurrent refreshes ran %d scans, want 1", callers, got)
	}
	var shared *Index
	for idx := range results {
		if shared == nil {
			shared = idx
		}
		if idx != shared {
			t.Error("callers got different indexes from one coalesced refresh")
		}
	}
	if shared.Generation != 2 {
		t.Errorf("generation = %d, want 2", shared.Generation)
	}
}

func newTestHolder(t *testing.T, root string) *Holder {
	t.Helper()
	holder, err := NewHolder(root, Options{Extensions: markdownExtensions, Logger: scanForTestLogger})
	if err != nil {
		t.Fatal(err)
	}
	return holder
}
