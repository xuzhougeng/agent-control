package registry

import (
	"path/filepath"
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := OpenStore(filepath.Join(dir, "registry.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestPublish_AssignsMonotonicVersions(t *testing.T) {
	st := newTestStore(t)
	v1, err := st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-01")
	if err != nil {
		t.Fatalf("publish 1: %v", err)
	}
	if v1 != 1 {
		t.Fatalf("first version = %d, want 1", v1)
	}
	v2, err := st.Publish(&Skill{Name: "x", Prompt: "p2"}, "ops-02")
	if err != nil {
		t.Fatalf("publish 2: %v", err)
	}
	if v2 != 2 {
		t.Fatalf("second version = %d, want 2", v2)
	}
}

func TestPublish_ConcurrentRaceProducesDistinctVersions(t *testing.T) {
	st := newTestStore(t)
	const N = 20
	var wg sync.WaitGroup
	versions := make(chan int, N)
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := st.Publish(&Skill{Name: "x", Prompt: "p"}, "ops-X")
			if err != nil {
				errs <- err
				return
			}
			versions <- v
		}()
	}
	wg.Wait()
	close(versions)
	close(errs)
	for e := range errs {
		t.Fatalf("publish error: %v", e)
	}
	seen := map[int]bool{}
	for v := range versions {
		if seen[v] {
			t.Fatalf("duplicate version %d", v)
		}
		seen[v] = true
	}
	if len(seen) != N {
		t.Fatalf("got %d distinct versions, want %d", len(seen), N)
	}
}
