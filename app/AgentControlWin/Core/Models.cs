using System.Text.Json.Serialization;
using System.Text.Json;

namespace AgentControlWin.Core;

// MARK: - Enums

public enum SessionType
{
    [JsonPropertyName("pty")] Pty,
    [JsonPropertyName("chat")] Chat,
}

public enum ChatRunState
{
    Idle,
    Running,
    Slow,
    Error,
}

// MARK: - REST Models (mirrors cc-control/internal/core/model.go)

public class Server
{
    [JsonPropertyName("server_id")] public string ServerId { get; set; } = "";
    [JsonPropertyName("hostname")] public string Hostname { get; set; } = "";
    [JsonPropertyName("tags")] public List<string>? Tags { get; set; }
    [JsonPropertyName("os")] public string? Os { get; set; }
    [JsonPropertyName("arch")] public string? Arch { get; set; }
    [JsonPropertyName("agent_version")] public string? AgentVersion { get; set; }
    [JsonPropertyName("last_seen_ms")] public long? LastSeenMs { get; set; }
    [JsonPropertyName("status")] public string Status { get; set; } = "";
    [JsonPropertyName("allow_roots")] public List<string>? AllowRoots { get; set; }
    [JsonPropertyName("claude_path")] public string? ClaudePath { get; set; }

    public bool IsOnline => Status == "online";

    /// <summary>
    /// True when the server is running the new in-process cc-agent runtime
    /// (vs the legacy cc-proxy PTY wrapper). Detected via the registered
    /// "cc-agent" tag or the synthetic claude_path="cc-agent-builtin".
    /// </summary>
    public bool IsCCAgent =>
        (Tags?.Contains("cc-agent") ?? false) || ClaudePath == "cc-agent-builtin";
}

public class Session
{
    [JsonPropertyName("session_id")] public string SessionId { get; set; } = "";
    [JsonPropertyName("server_id")] public string ServerId { get; set; } = "";
    [JsonPropertyName("session_type")] public string SessionTypeRaw { get; set; } = "pty";
    [JsonPropertyName("cwd")] public string Cwd { get; set; } = "";
    [JsonPropertyName("cmd")] public List<string>? Cmd { get; set; }
    [JsonPropertyName("env_keys")] public List<string>? EnvKeys { get; set; }
    [JsonPropertyName("status")] public string Status { get; set; } = "";
    [JsonPropertyName("created_by")] public string? CreatedBy { get; set; }
    [JsonPropertyName("created_at_ms")] public long? CreatedAtMs { get; set; }
    [JsonPropertyName("exit_code")] public int? ExitCode { get; set; }
    [JsonPropertyName("exit_reason")] public string? ExitReason { get; set; }
    [JsonPropertyName("awaiting_approval")] public bool AwaitingApproval { get; set; }
    [JsonPropertyName("pending_event_id")] public string? PendingEventId { get; set; }

    public bool IsRunning => Status == "running";
    public string ShortId => SessionId.Length >= 8 ? SessionId[..8] : SessionId;
    public bool IsChat => SessionTypeRaw == "chat";
    public bool IsPty => SessionTypeRaw == "pty";
    public SessionType SessionType => IsChat ? SessionType.Chat : SessionType.Pty;
}

public class SessionEvent
{
    [JsonPropertyName("event_id")] public string EventId { get; set; } = "";
    [JsonPropertyName("session_id")] public string SessionId { get; set; } = "";
    [JsonPropertyName("server_id")] public string ServerId { get; set; } = "";
    [JsonPropertyName("kind")] public string Kind { get; set; } = "";
    [JsonPropertyName("prompt_excerpt")] public string? PromptExcerpt { get; set; }
    [JsonPropertyName("actor")] public string? Actor { get; set; }
    [JsonPropertyName("ts_ms")] public long TsMs { get; set; }
    [JsonPropertyName("resolved")] public bool Resolved { get; set; }
    /// <summary>
    /// Non-empty when the approval came from a cc-agent's RemoteApprover (a
    /// destructive shell command). Empty for legacy cc-proxy PTY prompts.
    /// </summary>
    [JsonPropertyName("agent_request_id")] public string? AgentRequestId { get; set; }
    public bool IsFromCCAgent => !string.IsNullOrEmpty(AgentRequestId);
}

// MARK: - REST Response Wrappers

public class ServersResponse
{
    [JsonPropertyName("servers")] public List<Server> Servers { get; set; } = [];
}

public class SessionsResponse
{
    [JsonPropertyName("sessions")] public List<Session> Sessions { get; set; } = [];
}

public class ChatHistoryResponse
{
    [JsonPropertyName("messages")] public List<ChatMessage> Messages { get; set; } = [];
}

// MARK: - Chat Models

public class ChatImageSource
{
    [JsonPropertyName("type")] public string Type { get; set; } = "base64";
    [JsonPropertyName("media_type")] public string MediaType { get; set; } = "";
    [JsonPropertyName("data")] public string Data { get; set; } = "";
}

public class ChatContentPart
{
    [JsonPropertyName("type")] public string Type { get; set; } = "";
    [JsonPropertyName("text")] public string? Text { get; set; }
    [JsonPropertyName("source")] public ChatImageSource? Source { get; set; }
}

public class ChatMeta
{
    [JsonPropertyName("operations")] public List<string>? Operations { get; set; }
    [JsonPropertyName("content_parts")] public List<ChatContentPart>? ContentParts { get; set; }
    [JsonPropertyName("source")] public string? Source { get; set; }
    [JsonPropertyName("progress")] public bool? Progress { get; set; }
}

public class ChatMessage
{
    [JsonPropertyName("message_id")] public string MessageId { get; set; } = "";
    [JsonPropertyName("session_id")] public string SessionId { get; set; } = "";
    [JsonPropertyName("role")] public string Role { get; set; } = "assistant";
    [JsonPropertyName("content")] public string Content { get; set; } = "";
    [JsonPropertyName("meta")] public ChatMeta? Meta { get; set; }
    [JsonPropertyName("ts_ms")] public long TsMs { get; set; }

    public string UniqueId => $"{Role}:{MessageId}:{TsMs}";
    public bool IsUser => Role == "user";
    public bool IsAssistant => Role == "assistant";
}

// MARK: - WS Message Types

public record WsTermOut(string SessionId, byte[] Data, ulong Seq);

public record WsSessionUpdate(
    string SessionId,
    string Status,
    int? ExitCode,
    string? ExitReason,
    bool AwaitingApproval,
    string? PendingEventId,
    string? SessionType
);

// MARK: - WS Message Parser

public static class WsMessageParser
{
    public static void Parse(
        string text,
        Action<WsTermOut>? onTermOut,
        Action<SessionEvent>? onEvent,
        Action<WsSessionUpdate>? onSessionUpdate,
        Action<ChatMessage>? onChatMsg,
        Action<string>? onAttachOk,
        Action<string, string>? onError)
    {
        try
        {
            using var doc = JsonDocument.Parse(text);
            var root = doc.RootElement;
            if (!root.TryGetProperty("type", out var typeProp)) return;
            var type = typeProp.GetString();

            var sessionId = root.TryGetProperty("session_id", out var sidProp)
                ? sidProp.GetString() ?? "" : "";

            switch (type)
            {
                case "term_out":
                {
                    if (!root.TryGetProperty("data_b64", out var b64Prop)) return;
                    var b64 = b64Prop.GetString();
                    if (string.IsNullOrEmpty(b64)) return;
                    var bytes = Convert.FromBase64String(b64);
                    var seq = root.TryGetProperty("seq", out var seqProp)
                        ? seqProp.GetUInt64() : 0;
                    onTermOut?.Invoke(new WsTermOut(sessionId, bytes, seq));
                    break;
                }
                case "event":
                {
                    if (!root.TryGetProperty("data", out var dataProp)) return;
                    var ev = ParseSessionEvent(dataProp);
                    if (ev != null) onEvent?.Invoke(ev);
                    break;
                }
                case "session_update":
                {
                    if (!root.TryGetProperty("data", out var dataProp)) return;
                    var upd = ParseSessionUpdate(dataProp, sessionId);
                    onSessionUpdate?.Invoke(upd);
                    break;
                }
                case "chat_msg":
                {
                    if (!root.TryGetProperty("data", out var dataProp)) return;
                    var msg = JsonSerializer.Deserialize<ChatMessage>(dataProp.GetRawText());
                    if (msg != null) onChatMsg?.Invoke(msg);
                    break;
                }
                case "attach_ok":
                    onAttachOk?.Invoke(sessionId);
                    break;
                case "error":
                {
                    var message = root.TryGetProperty("data", out var dataProp)
                        && dataProp.TryGetProperty("message", out var msgProp)
                        ? msgProp.GetString() ?? "unknown error"
                        : "unknown error";
                    onError?.Invoke(sessionId, message);
                    break;
                }
            }
        }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine($"[ws] parse error: {ex.Message}");
        }
    }

    private static SessionEvent? ParseSessionEvent(JsonElement d)
    {
        if (!d.TryGetProperty("event_id", out var eid)) return null;
        if (!d.TryGetProperty("session_id", out var sid)) return null;
        if (!d.TryGetProperty("server_id", out var svid)) return null;
        if (!d.TryGetProperty("kind", out var kind)) return null;
        if (!d.TryGetProperty("ts_ms", out var tsMs)) return null;

        return new SessionEvent
        {
            EventId = eid.GetString() ?? "",
            SessionId = sid.GetString() ?? "",
            ServerId = svid.GetString() ?? "",
            Kind = kind.GetString() ?? "",
            PromptExcerpt = d.TryGetProperty("prompt_excerpt", out var pe) ? pe.GetString() : null,
            Actor = d.TryGetProperty("actor", out var ac) ? ac.GetString() : null,
            TsMs = tsMs.GetInt64(),
            Resolved = d.TryGetProperty("resolved", out var res) && res.GetBoolean(),
            AgentRequestId = d.TryGetProperty("agent_request_id", out var ari) ? ari.GetString() : null,
        };
    }

    private static WsSessionUpdate ParseSessionUpdate(JsonElement d, string fallbackId)
    {
        var sessionId = d.TryGetProperty("session_id", out var sid) ? sid.GetString() ?? fallbackId : fallbackId;
        var status = d.TryGetProperty("status", out var st) ? st.GetString() ?? "" : "";
        int? exitCode = d.TryGetProperty("exit_code", out var ec) ? ec.GetInt32() : null;
        string? exitReason = d.TryGetProperty("exit_reason", out var er) ? er.GetString() : null;
        bool awaitingApproval = d.TryGetProperty("awaiting_approval", out var aa) && aa.GetBoolean();
        string? pendingEventId = d.TryGetProperty("pending_event_id", out var peid) ? peid.GetString() : null;
        string? sessionType = d.TryGetProperty("session_type", out var stype) ? stype.GetString() : null;
        return new WsSessionUpdate(sessionId, status, exitCode, exitReason, awaitingApproval, pendingEventId, sessionType);
    }
}
