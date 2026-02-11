package main

import (
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"cc-agent/internal/agent"
	"cc-agent/internal/security"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})))
	hostname, _ := os.Hostname()
	var (
		controlURL     = flag.String("control-url", getenv("CONTROL_URL", "ws://127.0.0.1:18080/ws/agent"), "control-plane ws url")
		serverID       = flag.String("server-id", getenv("SERVER_ID", hostname), "stable server id")
		serverHost     = flag.String("hostname", hostname, "hostname shown in control plane")
		tagsCSV        = flag.String("tags", getenv("TAGS", ""), "comma-separated tags")
		allowRootsCSV  = flag.String("allow-root", getenv("ALLOW_ROOT", ""), "comma-separated allowed repo roots")
		claudePath     = flag.String("claude-path", getenv("CLAUDE_PATH", "claude-code"), "claude-code executable path")
		agentToken       = flag.String("agent-token", getenv("AGENT_TOKEN", "agent-dev-token"), "agent bearer token")
		tlsSkipVerify    = flag.Bool("tls-skip-verify", getenvBool("TLS_SKIP_VERIFY", false), "skip TLS cert verification (e.g. self-signed)")
		envAllowKeys     = flag.String("env-allow-keys", getenv("ENV_ALLOW_KEYS", ""), "comma-separated allowed env keys")
		envAllowPrefix = flag.String("env-allow-prefix", getenv("ENV_ALLOW_PREFIX", "CC_"), "allowed env key prefix")
	)
	flag.Parse()

	roots, err := security.NormalizeRoots(security.ParseCSV(*allowRootsCSV))
	if err != nil {
		slog.Error("invalid allow-root", "err", err)
		os.Exit(1)
	}
	allowedKeys := make(map[string]struct{})
	for _, k := range security.ParseCSV(*envAllowKeys) {
		allowedKeys[k] = struct{}{}
	}

	mgr := agent.NewSessionManager(agent.Config{
		ServerID:       *serverID,
		Hostname:       *serverHost,
		Tags:           security.ParseCSV(*tagsCSV),
		AllowRoots:     roots,
		ClaudePath:     *claudePath,
		EnvAllowKeys:   allowedKeys,
		EnvAllowPrefix: *envAllowPrefix,
	})

	url, err := agent.NormalizeWSURL(*controlURL)
	if err != nil {
		slog.Error("bad control-url", "err", err)
		os.Exit(1)
	}

	client := &agent.Client{
		URL:            url,
		Token:          *agentToken,
		HeartbeatEvery: 5 * time.Second,
		Manager:        mgr,
		TLSSkipVerify:  *tlsSkipVerify,
	}

	stop := make(chan struct{})
	go func() {
		if err := client.Run(stop); err != nil {
			slog.Error("agent stopped with error", "err", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	close(stop)
	time.Sleep(500 * time.Millisecond)
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}

func getenvBool(k string, fallback bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v == "1" || strings.EqualFold(v, "true") || v == "yes"
}
