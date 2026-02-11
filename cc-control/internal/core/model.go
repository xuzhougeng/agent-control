package core

type ServerStatus string

const (
	ServerOnline  ServerStatus = "online"
	ServerOffline ServerStatus = "offline"
)

type SessionStatus string

const (
	SessionStarting SessionStatus = "starting"
	SessionRunning  SessionStatus = "running"
	SessionStopping SessionStatus = "stopping"
	SessionExited   SessionStatus = "exited"
	SessionError    SessionStatus = "error"
)

type Server struct {
	ServerID     string       `json:"server_id"`
	Hostname     string       `json:"hostname"`
	Tags         []string     `json:"tags"`
	OS           string       `json:"os"`
	Arch         string       `json:"arch"`
	AgentVersion string       `json:"agent_version"`
	LastSeenMS   int64        `json:"last_seen_ms"`
	Status       ServerStatus `json:"status"`
	AllowRoots   []string     `json:"allow_roots,omitempty"`
	ClaudePath   string       `json:"claude_path,omitempty"`
}

type Session struct {
	SessionID         string        `json:"session_id"`
	ServerID          string        `json:"server_id"`
	Cwd               string        `json:"cwd"`
	Cmd               []string      `json:"cmd"`
	ResumeID          string        `json:"resume_id,omitempty"`
	EnvKeys           []string      `json:"env_keys"`
	Status            SessionStatus `json:"status"`
	CreatedBy         string        `json:"created_by"`
	CreatedAtMS       int64         `json:"created_at_ms"`
	ExitCode          *int          `json:"exit_code,omitempty"`
	ExitReason        string        `json:"exit_reason,omitempty"`
	AwaitingApproval  bool          `json:"awaiting_approval"`
	PendingEventID    string        `json:"pending_event_id,omitempty"`
	LatestAgentOutSeq uint64        `json:"latest_agent_out_seq"`
}

type SessionEvent struct {
	EventID    string `json:"event_id"`
	SessionID  string `json:"session_id"`
	ServerID   string `json:"server_id"`
	Kind       string `json:"kind"`
	PromptText string `json:"prompt_excerpt,omitempty"`
	Actor      string `json:"actor,omitempty"`
	TsMS       int64  `json:"ts_ms"`
	Resolved   bool   `json:"resolved"`
}

type StartSessionRequest struct {
	ServerID string            `json:"server_id"`
	Cwd      string            `json:"cwd"`
	ResumeID string            `json:"resume_id,omitempty"`
	Env      map[string]string `json:"env"`
	Cols     uint16            `json:"cols"`
	Rows     uint16            `json:"rows"`
}

type StopSessionRequest struct {
	GraceMS     int `json:"grace_ms"`
	KillAfterMS int `json:"kill_after_ms"`
}

type ActionRequest struct {
	Kind    string `json:"kind"`
	EventID string `json:"event_id,omitempty"`
}

type AgentRegister struct {
	ServerID     string   `json:"server_id"`
	Hostname     string   `json:"hostname"`
	Tags         []string `json:"tags"`
	OS           string   `json:"os"`
	Arch         string   `json:"arch"`
	AgentVersion string   `json:"agent_version"`
	AllowRoots   []string `json:"allow_roots"`
	ClaudePath   string   `json:"claude_path"`
}

type PTYExit struct {
	ExitCode *int   `json:"exit_code,omitempty"`
	Signal   string `json:"signal,omitempty"`
	Reason   string `json:"reason,omitempty"`
}
