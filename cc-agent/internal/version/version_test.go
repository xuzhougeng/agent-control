package version

import "testing"

func TestFormatIncludesInjectedVersionMetadata(t *testing.T) {
	got := Format("v0.7.3", "a8903aa9d10d361e4fe8c6ac50586af7b53cce83", "2026-05-08T23:31:49Z")
	want := "cc-agent v0.7.3 (a8903aa, 2026-05-08T23:31:49Z)"
	if got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

func TestFormatOmitsEmptyMetadataSeparators(t *testing.T) {
	got := Format("", "", "")
	want := "cc-agent dev"
	if got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}
