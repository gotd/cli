package filelock

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLockIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")

	// Separate Lock calls open separate file descriptions, so they exclude
	// each other just as separate processes do.
	var (
		mu       sync.Mutex
		inside   int
		maxInner int
	)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := Lock(path)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			inside++
			maxInner = max(maxInner, inside)
			mu.Unlock()

			time.Sleep(5 * time.Millisecond)

			mu.Lock()
			inside--
			mu.Unlock()
			if err := unlock(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if maxInner != 1 {
		t.Errorf("%d holders at once, want 1", maxInner)
	}
}

func TestLockReusable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	for range 3 {
		unlock, err := Lock(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := unlock(); err != nil {
			t.Fatal(err)
		}
	}
}
