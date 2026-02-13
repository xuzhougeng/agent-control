package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cc-control/internal/auth"
	"cc-control/internal/core"
	httpapi "cc-control/internal/http"
	"github.com/google/uuid"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})))
	var (
		addr                  = flag.String("addr", ":18080", "http listen address")
		uiDir                 = flag.String("ui-dir", "../ui", "static ui directory")
		agentToken            = flag.String("agent-token", getenv("AGENT_TOKEN", "agent-dev-token"), "agent bearer token")
		uiToken               = flag.String("ui-token", getenv("UI_TOKEN", "admin-dev-token"), "ui bearer token")
		adminToken            = flag.String("admin-token", getenv("ADMIN_TOKEN", ""), "admin bearer token (optional)")
		auditPath             = flag.String("audit-path", "./audit.jsonl", "audit jsonl path")
		ringBufferBytes       = flag.Int("ring-buffer-bytes", 128*1024, "session ring buffer size")
		offlineAfterSec       = flag.Int("offline-after-sec", 20, "mark server offline if no heartbeat")
		enablePromptDetection = flag.Bool("enable-prompt-detection", false, "enable heuristic prompt detection to emit approval_needed events (default: off)")
	)
	flag.Parse()

	cp, err := core.NewControlPlane(core.Config{
		RingBufferBytes:       *ringBufferBytes,
		OfflineAfter:          time.Duration(*offlineAfterSec) * time.Second,
		HeartbeatMS:           5000,
		AuditPath:             *auditPath,
		RateLimitPerMin:       1200,
		RateWindow:            time.Minute,
		DefaultGraceMS:        4000,
		DefaultKillMS:         9000,
		ApprovalBroadcast:     "all",
		EnablePromptDetection: *enablePromptDetection,
	})
	if err != nil {
		slog.Error("init control plane failed", "err", err)
		os.Exit(1)
	}
	defer cp.Close()

	tokenStore := auth.NewStore()
	defaultTenantID := ""
	if *agentToken != "" || *uiToken != "" {
		defaultTenantID = uuid.NewString()
	}
	if *uiToken != "" {
		if _, err := tokenStore.SeedToken(*uiToken, auth.TokenTypeUI, auth.RoleOwner, defaultTenantID, "legacy-ui"); err != nil {
			slog.Error("seed ui token failed", "err", err)
			os.Exit(1)
		}
	}
	if *agentToken != "" {
		if _, err := tokenStore.SeedToken(*agentToken, auth.TokenTypeAgent, "", defaultTenantID, "legacy-agent"); err != nil {
			slog.Error("seed agent token failed", "err", err)
			os.Exit(1)
		}
	}
	if *adminToken != "" {
		if _, err := tokenStore.SeedToken(*adminToken, auth.TokenTypeAdmin, "", "", "admin"); err != nil {
			slog.Error("seed admin token failed", "err", err)
			os.Exit(1)
		}
	}

	api := &httpapi.Server{
		CP:          cp,
		Tokens:      tokenStore,
		UIDir:       *uiDir,
		CheckOrigin: false,
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("cc-control listening", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("cc-control shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func getenv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}
