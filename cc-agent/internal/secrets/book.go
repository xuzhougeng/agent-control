// Package secrets is the operator password book.
//
// Tools request a named secret (sudo, mysql/prod, …) at execution time. The
// value is injected into the tool's runtime (askpass, env, client config)
// and must never appear in LLM context, tool results, logs, or audit trails.
package secrets

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"
)

var (
	ErrDenied      = errors.New("secret denied by operator")
	ErrNoPrompter  = errors.New("secret book has no operator prompter")
	ErrInvalidName = errors.New("invalid secret name")
)

// nameRE is conservative so names are safe as map keys and UI labels.
// Examples: sudo, mysql/prod, ssh/jump-host.
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9_./-]{0,63}$`)

// Request is what a tool sends when it needs a secret. Name is the book
// key; Reason and Command are shown to the operator and never stored
// alongside the secret value.
type Request struct {
	Name    string
	Reason  string
	Command string
	// Cache is how long a newly granted secret is kept. Zero means until
	// Drop or process exit.
	Cache time.Duration
}

// Prompter asks the operator for a secret. Implementations must not log
// the returned bytes.
type Prompter interface {
	Prompt(ctx context.Context, req Request) (secret []byte, granted bool, err error)
}

type entry struct {
	secret []byte
	exp    time.Time // zero = no expiry
}

// Book is an in-memory named secret store. Values are never persisted.
type Book struct {
	mu     sync.Mutex
	items  map[string]entry
	prompt Prompter
}

func NewBook(prompt Prompter) *Book {
	return &Book{items: map[string]entry{}, prompt: prompt}
}

func (b *Book) SetPrompter(p Prompter) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.prompt = p
	b.mu.Unlock()
}

func ValidName(name string) bool {
	return nameRE.MatchString(name)
}

// Get returns a copy of the named secret. On a miss it asks the operator
// (if a Prompter is attached) and optionally caches the result.
//
// The caller must Wipe the returned slice when finished.
func (b *Book) Get(ctx context.Context, req Request) ([]byte, error) {
	if b == nil {
		return nil, ErrNoPrompter
	}
	if !ValidName(req.Name) {
		return nil, ErrInvalidName
	}

	b.mu.Lock()
	if out, ok := b.cloneLocked(req.Name); ok {
		b.mu.Unlock()
		return out, nil
	}
	p := b.prompt
	b.mu.Unlock()

	if p == nil {
		return nil, ErrNoPrompter
	}
	secret, granted, err := p.Prompt(ctx, req)
	if err != nil {
		Wipe(secret)
		return nil, err
	}
	if !granted || len(secret) == 0 {
		Wipe(secret)
		return nil, ErrDenied
	}

	out := clone(secret)
	b.Put(req.Name, secret, req.Cache)
	Wipe(secret)
	return out, nil
}

// Put stores a copy of secret under name. The caller's slice is not retained.
func (b *Book) Put(name string, secret []byte, ttl time.Duration) {
	if b == nil || !ValidName(name) || len(secret) == 0 {
		return
	}
	e := entry{secret: clone(secret)}
	if ttl > 0 {
		e.exp = time.Now().Add(ttl)
	}
	b.mu.Lock()
	if old, ok := b.items[name]; ok {
		Wipe(old.secret)
	}
	b.items[name] = e
	b.mu.Unlock()
}

func (b *Book) Drop(name string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	if old, ok := b.items[name]; ok {
		Wipe(old.secret)
		delete(b.items, name)
	}
	b.mu.Unlock()
}

func (b *Book) Has(name string) bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.cloneLocked(name)
	return ok
}

// List returns stored names (not values), sorted.
func (b *Book) List() []string {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	out := make([]string, 0, len(b.items))
	for name, e := range b.items {
		if e.expired(now) {
			Wipe(e.secret)
			delete(b.items, name)
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (b *Book) cloneLocked(name string) ([]byte, bool) {
	e, ok := b.items[name]
	if !ok {
		return nil, false
	}
	if e.expired(time.Now()) {
		Wipe(e.secret)
		delete(b.items, name)
		return nil, false
	}
	return clone(e.secret), true
}

func (e entry) expired(now time.Time) bool {
	return !e.exp.IsZero() && !e.exp.After(now)
}

func clone(in []byte) []byte {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

// Wipe overwrites a secret slice. No-op on nil.
func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
