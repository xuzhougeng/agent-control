package core

import "testing"

func TestLooksLikeApprovalMenuPrompt_CreateFileMenu(t *testing.T) {
	prompt := `
Do you want to create unified_approval_test_abc?
❯ 1. Yes
  2. Yes, allow all edits during this session (shift+tab)
  3. No

Esc to cancel · Tab to amend
`
	if !looksLikeApprovalMenuPrompt(prompt) {
		t.Fatal("expected create-file approval menu to be recognized")
	}
}

func TestLooksLikeApprovalMenuPrompt_NumberedWithoutEscTab(t *testing.T) {
	prompt := `
Do you want to proceed?
1) Yes
2) Always allow access from this project
3) No
`
	if !looksLikeApprovalMenuPrompt(prompt) {
		t.Fatal("expected numbered approval menu to be recognized")
	}
}

func TestLooksLikeApprovalMenuPrompt_PlainYNPrompt(t *testing.T) {
	prompt := "Do you want to continue? [y/N]"
	if looksLikeApprovalMenuPrompt(prompt) {
		t.Fatal("plain y/n prompt should not be treated as menu")
	}
}
