// Package registry implements cc-control's team-private skill marketplace.
// It exposes HTTP endpoints for cc-agent and cc-web to publish, install, list,
// and browse skill kernels. Storage is a sqlite database independent of the
// existing session store.
package registry

// Skill is the wire format for a skill kernel. Mirrors cc-agent's
// internal/skills.Skill so JSON crosses the boundary unchanged.
type Skill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	Tools       []string `json:"tools"`
	Examples    []string `json:"examples"`
}

// StoredSkill is what the store returns: a Skill plus registry metadata.
type StoredSkill struct {
	Skill
	Version        int    `json:"version"`
	AuthorServerID string `json:"author_server_id"`
	CreatedAtUnix  int64  `json:"created_at_unix"`
	DeletedAtUnix  *int64 `json:"deleted_at_unix,omitempty"`
}

// Summary is a list-row shape: enough to render one row, no full body.
type Summary struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Version        int    `json:"version"`
	AuthorServerID string `json:"author_server_id"`
	CreatedAtUnix  int64  `json:"created_at_unix"`
}
