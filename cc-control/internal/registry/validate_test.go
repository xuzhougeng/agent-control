package registry

import "testing"

func TestValidate(t *testing.T) {
	knownTools := []string{"bash", "read", "write", "grep", "glob", "sysinfo", "proclist", "logtail"}
	cases := []struct {
		name      string
		s         Skill
		wantField string // empty = expect nil error
	}{
		{"happy path", Skill{Name: "nginx-triage", Prompt: "x", Tools: []string{"bash"}}, ""},
		{"empty name", Skill{Name: "", Prompt: "x"}, "name"},
		{"bad name regex (uppercase)", Skill{Name: "Nginx", Prompt: "x"}, "name"},
		{"bad name regex (starts digit)", Skill{Name: "1foo", Prompt: "x"}, "name"},
		{"too long name", Skill{Name: string(makeBytes('a', 80)), Prompt: "x"}, "name"},
		{"empty prompt", Skill{Name: "ok", Prompt: ""}, "prompt"},
		{"prompt too big", Skill{Name: "ok", Prompt: string(makeBytes('a', 33*1024))}, "prompt"},
		{"unknown tool", Skill{Name: "ok", Prompt: "x", Tools: []string{"kubectl"}}, "tools"},
		{"too many examples", Skill{Name: "ok", Prompt: "x", Examples: makeStrs(11)}, "examples"},
		{"example too long", Skill{Name: "ok", Prompt: "x", Examples: []string{string(makeBytes('a', 1025))}}, "examples"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(&tc.s, knownTools)
			if tc.wantField == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error on field %q, got nil", tc.wantField)
			}
			ve, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("want *ValidationError, got %T", err)
			}
			if ve.Field != tc.wantField {
				t.Fatalf("want field %q, got %q", tc.wantField, ve.Field)
			}
		})
	}
}

func makeBytes(c byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return b
}

func makeStrs(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "x"
	}
	return out
}
