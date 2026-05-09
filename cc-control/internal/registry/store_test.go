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

func TestLatest_ReturnsHighestNotDeleted(t *testing.T) {
	st := newTestStore(t)
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p1"}, "a")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p2"}, "a")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p3"}, "a")
	got, err := st.Latest("x")
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if got.Version != 3 || got.Prompt != "p3" {
		t.Fatalf("got version=%d prompt=%q, want 3/p3", got.Version, got.Prompt)
	}
}

func TestGet_Versioned(t *testing.T) {
	st := newTestStore(t)
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p1"}, "a")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p2"}, "a")
	got, err := st.Get("x", 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Prompt != "p1" {
		t.Fatalf("Get(x,1).Prompt=%q want p1", got.Prompt)
	}
}

func TestGet_NotFound(t *testing.T) {
	st := newTestStore(t)
	_, err := st.Get("nope", 0)
	if err != ErrNotFound {
		t.Fatalf("Get(nope) err = %v, want ErrNotFound", err)
	}
}

func TestList_FiltersByQuery(t *testing.T) {
	st := newTestStore(t)
	mustPublish(t, st, &Skill{Name: "nginx-triage", Prompt: "p", Description: "Triage nginx"}, "a")
	mustPublish(t, st, &Skill{Name: "k8s-debug", Prompt: "p", Description: "Debug k8s"}, "a")
	all, err := st.List("")
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List() len=%d, want 2", len(all))
	}
	hits, err := st.List("nginx")
	if err != nil {
		t.Fatalf("List(nginx): %v", err)
	}
	if len(hits) != 1 || hits[0].Name != "nginx-triage" {
		t.Fatalf("List(nginx) = %+v, want [nginx-triage]", hits)
	}
}

func TestHistory_AllVersionsOldestFirst(t *testing.T) {
	st := newTestStore(t)
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p1"}, "a")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p2"}, "b")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p3"}, "c")
	hist, err := st.History("x")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("len=%d, want 3", len(hist))
	}
	for i, h := range hist {
		if h.Version != i+1 {
			t.Fatalf("hist[%d].Version=%d", i, h.Version)
		}
	}
}

func TestSoftDelete_HidesFromListAndLatest(t *testing.T) {
	st := newTestStore(t)
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p1"}, "a")
	mustPublish(t, st, &Skill{Name: "x", Prompt: "p2"}, "a")
	if err := st.SoftDelete("x", 2, "admin"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	got, err := st.Latest("x")
	if err != nil {
		t.Fatalf("Latest after delete: %v", err)
	}
	if got.Version != 1 {
		t.Fatalf("Latest.Version=%d, want 1", got.Version)
	}
	hits, err := st.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(hits) != 1 || hits[0].Version != 1 {
		t.Fatalf("List = %+v, want one row at v1", hits)
	}
}

func mustPublish(t *testing.T, st *Store, s *Skill, author string) int {
	t.Helper()
	v, err := st.Publish(s, author)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	return v
}
