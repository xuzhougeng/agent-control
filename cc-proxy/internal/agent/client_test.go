package agent

import "testing"

func TestNormalizeWSURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "already ws", in: "ws://127.0.0.1:18080/ws/agent", want: "ws://127.0.0.1:18080/ws/agent"},
		{name: "already wss", in: "wss://example.com/ws/agent", want: "wss://example.com/ws/agent"},
		{name: "http to ws", in: "http://127.0.0.1:18080/ws/agent", want: "ws://127.0.0.1:18080/ws/agent"},
		{name: "https to wss", in: "https://example.com/ws/agent", want: "wss://example.com/ws/agent"},
		{name: "invalid", in: "://bad-url", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeWSURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeWSURL(%q)=%q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
