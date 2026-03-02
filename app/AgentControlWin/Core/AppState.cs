using System.Collections.ObjectModel;
using System.ComponentModel;
using System.Runtime.CompilerServices;
using Microsoft.UI.Dispatching;

namespace AgentControlWin.Core;

public enum AppDetailPage { Terminal, Chat }

public class AppState : INotifyPropertyChanged
{
    public event PropertyChangedEventHandler? PropertyChanged;

    private void OnPropChanged([CallerMemberName] string? name = null)
        => PropertyChanged?.Invoke(this, new PropertyChangedEventArgs(name));

    // MARK: - Published state

    public ObservableCollection<Server> Servers { get; } = [];
    public ObservableCollection<Session> Sessions { get; } = [];
    public Dictionary<string, SessionEvent> Approvals { get; } = [];

    private string? _selectedServerId;
    public string? SelectedServerId
    {
        get => _selectedServerId;
        set { _selectedServerId = value; OnPropChanged(); OnPropChanged(nameof(PendingApprovals)); }
    }

    private string? _selectedSessionId;
    public string? SelectedSessionId
    {
        get => _selectedSessionId;
        set { _selectedSessionId = value; OnPropChanged(); }
    }

    private string? _selectedChatSessionId;
    public string? SelectedChatSessionId
    {
        get => _selectedChatSessionId;
        set { _selectedChatSessionId = value; OnPropChanged(); }
    }

    private AppDetailPage _selectedPage = AppDetailPage.Terminal;
    public AppDetailPage SelectedPage
    {
        get => _selectedPage;
        set { _selectedPage = value; OnPropChanged(); }
    }

    public ObservableCollection<ChatMessage> ChatMessages { get; } = [];

    private ChatRunState _chatRunState = ChatRunState.Idle;
    public ChatRunState ChatRunState
    {
        get => _chatRunState;
        private set { _chatRunState = value; OnPropChanged(); OnPropChanged(nameof(ChatRunStateText)); }
    }

    private string _chatRunStateDetail = "";
    public string ChatRunStateText => _chatRunStateDetail;

    private bool _wsConnected;
    public bool WsConnected
    {
        get => _wsConnected;
        private set { _wsConnected = value; OnPropChanged(); }
    }

    private string? _connectionHint;
    public string? ConnectionHint
    {
        get => _connectionHint;
        private set { _connectionHint = value; OnPropChanged(); }
    }

    // MARK: - Computed

    public IEnumerable<SessionEvent> PendingApprovals =>
        Approvals.Values.Where(e => !e.Resolved).OrderByDescending(e => e.TsMs);

    // MARK: - Terminal bridge (cols/rows for resize)

    public int TermCols { get; set; } = 120;
    public int TermRows { get; set; } = 30;

    // Event fired when terminal data arrives — TerminalPage subscribes
    public event Action<string, byte[]>? OnTermOut;

    // MARK: - Subsystems

    public readonly ApiClient ApiClient = new();
    private WsClient _wsClient = null!;
    private readonly DispatcherQueue _dispatcher;
    private bool _didStart;
    private CancellationTokenSource? _sessionRefreshCts;
    private int _chatPendingTurns;
    private CancellationTokenSource? _chatSlowCts;

    public AppState(DispatcherQueue dispatcher)
    {
        _dispatcher = dispatcher;
    }

    // MARK: - Startup

    public void Start()
    {
        LoadConfig();
        _wsClient = new WsClient(_dispatcher);
        _wsClient.Configure(
            AppSettings.GetString("baseUrl", "http://127.0.0.1:18080")!,
            CredentialHelper.LoadToken() ?? "admin-dev-token",
            AppSettings.GetBool("skipTlsVerify"));

        _wsClient.OnTermOut += HandleTermOut;
        _wsClient.OnEvent += HandleEvent;
        _wsClient.OnSessionUpdate += HandleSessionUpdate;
        _wsClient.OnChatMsg += HandleChatMsg;
        _wsClient.OnAttachOk += HandleAttachOk;
        _wsClient.OnError += HandleWsError;
        _wsClient.OnConnectionChanged += connected =>
        {
            WsConnected = connected;
            if (connected) OnConnectedReattach();
        };

        if (_didStart) return;
        _didStart = true;

        _wsClient.Connect();
        _ = FetchServersAsync();
        _ = FetchSessionsAsync();
    }

    private void LoadConfig()
    {
        var baseUrl = AppSettings.GetString("baseUrl", "http://127.0.0.1:18080")!;
        var token = CredentialHelper.LoadToken() ?? "admin-dev-token";
        var skipTls = AppSettings.GetBool("skipTlsVerify");
        ApiClient.Configure(baseUrl, token, skipTls);
    }

    // MARK: - Config

    public static bool IsValidBaseUrl(string raw)
    {
        var value = raw.Trim();
        if (!Uri.TryCreate(value, UriKind.Absolute, out var uri)) return false;
        if (uri.Scheme != "http" && uri.Scheme != "https") return false;
        if (string.IsNullOrEmpty(uri.Host)) return false;
        return string.IsNullOrEmpty(uri.AbsolutePath) || uri.AbsolutePath == "/";
    }

    public void SaveConfig(string baseUrl, string token, bool skipTlsVerify)
    {
        if (!IsValidBaseUrl(baseUrl)) return;
        AppSettings.Set("baseUrl", baseUrl);
        AppSettings.Set("skipTlsVerify", skipTlsVerify);
        CredentialHelper.SaveToken(token);
        ApiClient.Configure(baseUrl, token, skipTlsVerify);
        _wsClient?.Configure(baseUrl, token, skipTlsVerify);
        _wsClient?.Disconnect();
        _wsClient?.Connect();
        _ = FetchServersAsync();
        _ = FetchSessionsAsync();
    }

    // MARK: - REST

    public async Task FetchServersAsync()
    {
        try
        {
            var servers = await ApiClient.GetServersAsync();
            _dispatcher.TryEnqueue(() =>
            {
                Servers.Clear();
                foreach (var s in servers) Servers.Add(s);
                if (SelectedServerId == null && Servers.Count > 0)
                    SelectedServerId = Servers[0].ServerId;
            });
        }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine($"[api] fetchServers: {ex.Message}");
        }
    }

    public async Task FetchSessionsAsync()
    {
        try
        {
            var sessions = await ApiClient.GetSessionsAsync(SelectedServerId);
            _dispatcher.TryEnqueue(() =>
            {
                Sessions.Clear();
                foreach (var s in sessions) Sessions.Add(s);
                NormalizeSessionSelection();
            });
        }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine($"[api] fetchSessions: {ex.Message}");
        }
    }

    public async Task CreateSessionAsync(string cwd, string? sessionId, Dictionary<string, string> env)
    {
        if (SelectedServerId == null) return;
        try
        {
            var sess = await ApiClient.CreateSessionAsync(
                SelectedServerId, cwd, sessionId, env, SessionType.Pty, TermCols, TermRows);
            await FetchSessionsAsync();
            _dispatcher.TryEnqueue(() => AttachSession(sess.SessionId));
        }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine($"[api] createSession: {ex.Message}");
        }
    }

    public async Task CreateChatSessionAsync(string cwd, string? sessionId, Dictionary<string, string> env)
    {
        if (SelectedServerId == null) return;
        try
        {
            var sess = await ApiClient.CreateSessionAsync(
                SelectedServerId, cwd, sessionId, env, SessionType.Chat);
            await FetchSessionsAsync();
            await AttachChatSessionAsync(sess.SessionId);
        }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine($"[api] createChatSession: {ex.Message}");
        }
    }

    public async Task StopSessionAsync(string sessionId)
    {
        try
        {
            await ApiClient.StopSessionAsync(sessionId);
            await FetchSessionsAsync();
        }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine($"[api] stopSession: {ex.Message}");
        }
    }

    public async Task DeleteSessionAsync(string sessionId)
    {
        try
        {
            await ApiClient.DeleteSessionAsync(sessionId);
            _dispatcher.TryEnqueue(() =>
            {
                if (SelectedSessionId == sessionId) { SelectedSessionId = null; }
                if (SelectedChatSessionId == sessionId) ResetChatSelection();
                var toRemove = Approvals.Where(kv => kv.Value.SessionId == sessionId).Select(kv => kv.Key).ToList();
                foreach (var k in toRemove) Approvals.Remove(k);
            });
            await FetchSessionsAsync();
        }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine($"[api] deleteSession: {ex.Message}");
        }
    }

    public async Task FetchChatHistoryAsync(string sessionId)
    {
        if (string.IsNullOrEmpty(sessionId)) return;
        try
        {
            var msgs = await ApiClient.GetChatHistoryAsync(sessionId);
            _dispatcher.TryEnqueue(() =>
            {
                if (SelectedChatSessionId != sessionId) return;
                ChatMessages.Clear();
                foreach (var m in msgs) ChatMessages.Add(m);
            });
        }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine($"[api] fetchChatHistory: {ex.Message}");
        }
    }

    public async Task SwitchSessionToChatAsync(string sessionId)
    {
        if (string.IsNullOrEmpty(sessionId)) return;
        await ApiClient.SwitchSessionAsync(sessionId, SessionType.Chat);
        await FetchSessionsAsync();
        await AttachChatSessionAsync(sessionId);
    }

    public async Task SwitchSessionToPtyAsync(string sessionId)
    {
        if (string.IsNullOrEmpty(sessionId)) return;
        await ApiClient.SwitchSessionAsync(sessionId, SessionType.Pty, null, TermCols, TermRows);
        await FetchSessionsAsync();
        _dispatcher.TryEnqueue(() => AttachSession(sessionId));
    }

    public async Task ApplyChatPermissionsAsync(string sessionId, string permissionMode,
        string allowedTools, string disallowedTools)
    {
        if (string.IsNullOrEmpty(sessionId)) return;
        var env = new Dictionary<string, string>();
        var mode = permissionMode.Trim();
        var allowed = allowedTools.Trim();
        var disallowed = disallowedTools.Trim();
        if (!string.IsNullOrEmpty(mode)) env["CC_CLAUDE_PERMISSION_MODE"] = mode;
        if (!string.IsNullOrEmpty(allowed)) env["CC_CLAUDE_ALLOWED_TOOLS"] = allowed;
        if (!string.IsNullOrEmpty(disallowed)) env["CC_CLAUDE_DISALLOWED_TOOLS"] = disallowed;
        await ApiClient.SwitchSessionAsync(sessionId, SessionType.Chat, env);
        await FetchSessionsAsync();
        await AttachChatSessionAsync(sessionId);
    }

    // MARK: - WS Actions

    public void AttachSession(string sessionId)
    {
        SelectedSessionId = sessionId;
        SelectedPage = AppDetailPage.Terminal;
        // Clear terminal via event
        OnTermOut?.Invoke(sessionId, Array.Empty<byte>()); // signal clear (page handles empty)
        _wsClient?.SendAttach(sessionId);
        _wsClient?.SendResize(sessionId, TermCols, TermRows);
        ScheduleResizeReplay(sessionId);
    }

    public async Task AttachChatSessionAsync(string sessionId)
    {
        if (string.IsNullOrEmpty(sessionId)) return;
        _dispatcher.TryEnqueue(() =>
        {
            SelectedChatSessionId = sessionId;
            SelectedPage = AppDetailPage.Chat;
            ChatMessages.Clear();
            _chatPendingTurns = 0;
            CancelSlowTransition();
            SetChatRunState(ChatRunState.Idle, "");
        });
        _wsClient?.SendAttach(sessionId);
        await FetchChatHistoryAsync(sessionId);
    }

    public bool SendChatMessage(string text, List<ChatContentPart> attachments)
    {
        var sid = SelectedChatSessionId;
        if (string.IsNullOrEmpty(sid))
        {
            SetChatRunState(ChatRunState.Error, "Select or create a chat session first.");
            return false;
        }
        var parts = new List<ChatContentPart>();
        var trimmed = text.Trim();
        if (!string.IsNullOrEmpty(trimmed))
            parts.Add(new ChatContentPart { Type = "text", Text = trimmed });
        parts.AddRange(attachments);
        if (parts.Count == 0) return false;
        if (!WsConnected)
        {
            SetChatRunState(ChatRunState.Error, "WebSocket disconnected.");
            return false;
        }
        _wsClient.SendChatIn(sid, trimmed, parts);
        BeginPendingChatTurn();
        return true;
    }

    public void SendTerminalInput(byte[] bytes)
    {
        if (string.IsNullOrEmpty(SelectedSessionId)) return;
        _wsClient?.SendTermIn(SelectedSessionId, Convert.ToBase64String(bytes));
    }

    public void SendResize(int cols, int rows)
    {
        if (string.IsNullOrEmpty(SelectedSessionId)) return;
        TermCols = cols;
        TermRows = rows;
        _wsClient?.SendResize(SelectedSessionId, cols, rows);
    }

    public void SendAction(string sessionId, string kind)
    {
        _wsClient?.SendAction(sessionId, kind);
    }

    // MARK: - WS Handlers

    private void HandleTermOut(WsTermOut msg)
    {
        if (msg.SessionId == SelectedSessionId)
            OnTermOut?.Invoke(msg.SessionId, msg.Data);
    }

    private void HandleEvent(SessionEvent ev)
    {
        if (ev.Kind == "approval_needed")
        {
            Approvals[ev.EventId] = ev;
            OnPropChanged(nameof(PendingApprovals));
        }
    }

    private void HandleSessionUpdate(WsSessionUpdate upd)
    {
        var idx = -1;
        for (int i = 0; i < Sessions.Count; i++)
        {
            if (Sessions[i].SessionId == upd.SessionId) { idx = i; break; }
        }
        if (idx >= 0)
        {
            var old = Sessions[idx];
            var patched = new Session
            {
                SessionId = old.SessionId,
                ServerId = old.ServerId,
                SessionTypeRaw = upd.SessionType ?? old.SessionTypeRaw,
                Cwd = old.Cwd,
                Cmd = old.Cmd,
                EnvKeys = old.EnvKeys,
                Status = string.IsNullOrEmpty(upd.Status) ? old.Status : upd.Status,
                CreatedBy = old.CreatedBy,
                CreatedAtMs = old.CreatedAtMs,
                ExitCode = upd.ExitCode ?? old.ExitCode,
                ExitReason = upd.ExitReason ?? old.ExitReason,
                AwaitingApproval = upd.AwaitingApproval,
                PendingEventId = upd.PendingEventId ?? old.PendingEventId,
            };
            Sessions[idx] = patched;
        }
        if (upd.SessionId == SelectedChatSessionId)
        {
            if (upd.Status is "error" or "exited")
                FailPendingChatTurns(upd.ExitReason ?? $"session {upd.Status}");
        }
        if (!upd.AwaitingApproval)
        {
            foreach (var kv in Approvals.Where(kv => kv.Value.SessionId == upd.SessionId && !kv.Value.Resolved).ToList())
            {
                Approvals[kv.Key] = new SessionEvent
                {
                    EventId = kv.Value.EventId, SessionId = kv.Value.SessionId,
                    ServerId = kv.Value.ServerId, Kind = kv.Value.Kind,
                    PromptExcerpt = kv.Value.PromptExcerpt, Actor = kv.Value.Actor,
                    TsMs = kv.Value.TsMs, Resolved = true,
                };
                OnPropChanged(nameof(PendingApprovals));
            }
        }
        DebouncedFetchSessions();
    }

    private void HandleChatMsg(ChatMessage msg)
    {
        if (msg.SessionId == SelectedChatSessionId)
        {
            ChatMessages.Add(msg);
            if (msg.IsAssistant && !(msg.Meta?.Progress == true))
                CompletePendingChatTurn();
        }
    }

    private void HandleAttachOk(string sessionId)
    {
        if (sessionId == SelectedSessionId)
            ScheduleResizeReplay(sessionId);
    }

    private void HandleWsError(string sessionId, string message)
    {
        System.Diagnostics.Debug.WriteLine($"[ws] error session={sessionId}: {message}");
        if (sessionId == SelectedChatSessionId)
            FailPendingChatTurns(message);
    }

    // MARK: - Helpers

    private void OnConnectedReattach()
    {
        if (!string.IsNullOrEmpty(SelectedSessionId))
        {
            _wsClient?.SendAttach(SelectedSessionId);
            _wsClient?.SendResize(SelectedSessionId, TermCols, TermRows);
            ScheduleResizeReplay(SelectedSessionId);
        }
        if (!string.IsNullOrEmpty(SelectedChatSessionId))
            _wsClient?.SendAttach(SelectedChatSessionId);
    }

    private void NormalizeSessionSelection()
    {
        if (SelectedSessionId != null && !Sessions.Any(s => s.SessionId == SelectedSessionId && s.IsPty))
            SelectedSessionId = null;
        if (SelectedChatSessionId != null && !Sessions.Any(s => s.SessionId == SelectedChatSessionId && s.IsChat))
            ResetChatSelection();
    }

    private void ResetChatSelection()
    {
        SelectedChatSessionId = null;
        ChatMessages.Clear();
        _chatPendingTurns = 0;
        CancelSlowTransition();
        SetChatRunState(ChatRunState.Idle, "");
    }

    private void DebouncedFetchSessions()
    {
        _sessionRefreshCts?.Cancel();
        _sessionRefreshCts = new CancellationTokenSource();
        var cts = _sessionRefreshCts;
        Task.Delay(1000, cts.Token).ContinueWith(t =>
        {
            if (!t.IsCanceled) _ = FetchSessionsAsync();
        });
    }

    private void ScheduleResizeReplay(string sessionId)
    {
        var delays = new[] { 0, 250, 500, 1000, 2000 };
        _ = Task.Run(async () =>
        {
            foreach (var d in delays)
            {
                if (d > 0) await Task.Delay(d);
                if (SelectedSessionId != sessionId) return;
                _dispatcher.TryEnqueue(() => _wsClient?.SendResize(sessionId, TermCols, TermRows));
            }
        });
    }

    // MARK: - Chat run state

    private void BeginPendingChatTurn()
    {
        _chatPendingTurns++;
        SetChatRunState(ChatRunState.Running,
            _chatPendingTurns > 1
                ? $"Running... ({_chatPendingTurns} requests pending)"
                : "Running... waiting for assistant response");
        if (_chatPendingTurns == 1) ScheduleSlowTransition();
    }

    private void CompletePendingChatTurn()
    {
        if (_chatPendingTurns <= 0) return;
        _chatPendingTurns--;
        if (_chatPendingTurns == 0) { CancelSlowTransition(); SetChatRunState(ChatRunState.Idle, ""); return; }
        SetChatRunState(ChatRunState.Running, $"Running... ({_chatPendingTurns} requests pending)");
    }

    private void FailPendingChatTurns(string reason)
    {
        if (_chatPendingTurns <= 0) return;
        _chatPendingTurns = 0;
        CancelSlowTransition();
        SetChatRunState(ChatRunState.Error, $"Execution failed: {reason}");
    }

    private void ScheduleSlowTransition()
    {
        CancelSlowTransition();
        _chatSlowCts = new CancellationTokenSource();
        var cts = _chatSlowCts;
        Task.Delay(12000, cts.Token).ContinueWith(t =>
        {
            if (t.IsCanceled) return;
            _dispatcher.TryEnqueue(() =>
            {
                if (_chatPendingTurns > 0)
                    SetChatRunState(ChatRunState.Slow, "Still running... this is taking longer than usual");
            });
        });
    }

    private void CancelSlowTransition()
    {
        _chatSlowCts?.Cancel();
        _chatSlowCts = null;
    }

    private void SetChatRunState(ChatRunState state, string detail)
    {
        _chatRunState = state;
        _chatRunStateDetail = detail;
        OnPropChanged(nameof(ChatRunState));
        OnPropChanged(nameof(ChatRunStateText));
    }
}
