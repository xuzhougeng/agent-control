package secrets

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakePrompt struct {
	secret  []byte
	granted bool
	err     error
	calls   int
	last    Request
}

func (f *fakePrompt) Prompt(_ context.Context, req Request) ([]byte, bool, error) {
	f.calls++
	f.last = req
	if f.err != nil {
		return nil, false, f.err
	}
	return clone(f.secret), f.granted, nil
}

func TestGetCachesUntilDrop(t *testing.T) {
	p := &fakePrompt{secret: []byte("hunter2"), granted: true}
	b := NewBook(p)
	got, err := b.Get(context.Background(), Request{Name: "sudo", Reason: "auth"})
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	if string(got) != "hunter2" {
		t.Fatalf("got %q", got)
	}
	Wipe(got)
	got2, err := b.Get(context.Background(), Request{Name: "sudo"})
	if err != nil {
		t.Fatalf("second get: %v", err)
	}
	if string(got2) != "hunter2" {
		t.Fatalf("cached %q", got2)
	}
	if p.calls != 1 {
		t.Fatalf("prompter calls = %d, want 1", p.calls)
	}
	b.Drop("sudo")
	p.secret = []byte("other")
	got3, err := b.Get(context.Background(), Request{Name: "sudo"})
	if err != nil {
		t.Fatalf("after drop: %v", err)
	}
	if string(got3) != "other" {
		t.Fatalf("after drop got %q", got3)
	}
	if p.calls != 2 {
		t.Fatalf("prompter calls after drop = %d, want 2", p.calls)
	}
}

func TestGetTTLExpires(t *testing.T) {
	p := &fakePrompt{secret: []byte("x"), granted: true}
	b := NewBook(p)
	if _, err := b.Get(context.Background(), Request{Name: "sudo", Cache: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if b.Has("sudo") {
		t.Fatal("expected ttl expiry")
	}
}

func TestGetDenied(t *testing.T) {
	b := NewBook(&fakePrompt{granted: false})
	_, err := b.Get(context.Background(), Request{Name: "sudo"})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("err = %v, want ErrDenied", err)
	}
}

func TestGetNoPrompter(t *testing.T) {
	b := NewBook(nil)
	_, err := b.Get(context.Background(), Request{Name: "mysql/prod"})
	if !errors.Is(err, ErrNoPrompter) {
		t.Fatalf("err = %v, want ErrNoPrompter", err)
	}
}

func TestInvalidName(t *testing.T) {
	b := NewBook(&fakePrompt{secret: []byte("x"), granted: true})
	for _, name := range []string{"", "SUDO", "../etc", "a b", "sudo;id"} {
		if _, err := b.Get(context.Background(), Request{Name: name}); !errors.Is(err, ErrInvalidName) {
			t.Errorf("name %q: err=%v, want ErrInvalidName", name, err)
		}
	}
}

func TestListHidesValues(t *testing.T) {
	b := NewBook(nil)
	b.Put("sudo", []byte("secret-value"), 0)
	b.Put("mysql/prod", []byte("also-secret"), 0)
	names := b.List()
	if len(names) != 2 || names[0] != "mysql/prod" || names[1] != "sudo" {
		t.Fatalf("list = %v", names)
	}
	for _, n := range names {
		if n == "secret-value" || n == "also-secret" {
			t.Fatal("list leaked a value")
		}
	}
}

func TestWipe(t *testing.T) {
	b := []byte("abc")
	Wipe(b)
	for _, c := range b {
		if c != 0 {
			t.Fatalf("wipe left %q", b)
		}
	}
}
