using System.Net.WebSockets;
using System.Text;
using System.Text.Json;
using Microsoft.UI.Dispatching;

namespace AgentControlWin.Core;

public class WsClient
{
    private const int ProtocolVersion = 1;

    private string _baseUrl = "";
    private string _token = "";
    private bool _skipTlsVerify;
    private ClientWebSocket? _ws;
    private CancellationTokenSource? _cts;
    private bool _shouldReconnect;
    private double _reconnectDelay = 1.0;
    private readonly DispatcherQueue _dispatcher;

    public event Action<WsTermOut>? OnTermOut;
    public event Action<SessionEvent>? OnEvent;
    public event Action<WsSessionUpdate>? OnSessionUpdate;
    public event Action<ChatMessage>? OnChatMsg;
    public event Action<string>? OnAttachOk;
    public event Action<string, string>? OnError;
    public event Action<bool>? OnConnectionChanged;

    public WsClient(DispatcherQueue dispatcher)
    {
        _dispatcher = dispatcher;
    }

    public void Configure(string baseUrl, string token, bool skipTlsVerify = false)
    {
        _baseUrl = baseUrl.TrimEnd('/');
        _token = token;
        _skipTlsVerify = skipTlsVerify;
    }

    public void Connect()
    {
        if (_ws?.State == WebSocketState.Open || _ws?.State == WebSocketState.Connecting) return;
        _shouldReconnect = true;
        _ = ConnectAsync();
    }

    public void Disconnect()
    {
        _shouldReconnect = false;
        _cts?.Cancel();
        _ws?.Abort();
        _ws = null;
    }

    private string BuildWsUrl()
    {
        var scheme = _baseUrl.StartsWith("https", StringComparison.OrdinalIgnoreCase) ? "wss" : "ws";
        var host = _baseUrl
            .Replace("https://", "")
            .Replace("http://", "");
        var encodedToken = Uri.EscapeDataString(_token);
        return $"{scheme}://{host}/ws/client?token={encodedToken}&v={ProtocolVersion}";
    }

    private async Task ConnectAsync()
    {
        _cts?.Cancel();
        _cts = new CancellationTokenSource();
        var token = _cts.Token;

        try
        {
            var url = BuildWsUrl();
            _ws = new ClientWebSocket();
            _ws.Options.SetRequestHeader("Authorization", $"Bearer {_token}");

            // Note: ClientWebSocket doesn't support per-connection TLS bypass easily.
            // For _skipTlsVerify in production builds, a custom handler would be needed.

            await _ws.ConnectAsync(new Uri(url), token);

            _reconnectDelay = 1.0;
            _dispatcher.TryEnqueue(() => OnConnectionChanged?.Invoke(true));

            await ReceiveLoopAsync(token);
        }
        catch (OperationCanceledException)
        {
            // intentional disconnect
        }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine($"[ws] connect error: {ex.Message}");
            _dispatcher.TryEnqueue(() => OnConnectionChanged?.Invoke(false));
            ScheduleReconnect();
        }
    }

    private async Task ReceiveLoopAsync(CancellationToken token)
    {
        var buffer = new byte[1024 * 64];
        var sb = new StringBuilder();
        try
        {
            while (_ws?.State == WebSocketState.Open && !token.IsCancellationRequested)
            {
                sb.Clear();
                WebSocketReceiveResult result;
                do
                {
                    result = await _ws.ReceiveAsync(new ArraySegment<byte>(buffer), token);
                    if (result.MessageType == WebSocketMessageType.Close)
                    {
                        _dispatcher.TryEnqueue(() => OnConnectionChanged?.Invoke(false));
                        ScheduleReconnect();
                        return;
                    }
                    sb.Append(Encoding.UTF8.GetString(buffer, 0, result.Count));
                }
                while (!result.EndOfMessage);

                var text = sb.ToString();
                _dispatcher.TryEnqueue(() =>
                {
                    WsMessageParser.Parse(text,
                        t => OnTermOut?.Invoke(t),
                        e => OnEvent?.Invoke(e),
                        u => OnSessionUpdate?.Invoke(u),
                        m => OnChatMsg?.Invoke(m),
                        s => OnAttachOk?.Invoke(s),
                        (sid, msg) => OnError?.Invoke(sid, msg));
                });
            }
        }
        catch (OperationCanceledException) { }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine($"[ws] receive error: {ex.Message}");
            _dispatcher.TryEnqueue(() => OnConnectionChanged?.Invoke(false));
            ScheduleReconnect();
        }
    }

    private void ScheduleReconnect()
    {
        if (!_shouldReconnect) return;
        var delay = _reconnectDelay;
        _reconnectDelay = Math.Min(_reconnectDelay * 2, 30.0);
        System.Diagnostics.Debug.WriteLine($"[ws] reconnecting in {delay}s");
        Task.Delay(TimeSpan.FromSeconds(delay)).ContinueWith(_ =>
        {
            if (_shouldReconnect) _ = ConnectAsync();
        });
    }

    private void Send(string json)
    {
        if (_ws?.State != WebSocketState.Open) return;
        var bytes = Encoding.UTF8.GetBytes(json);
        _ = _ws.SendAsync(new ArraySegment<byte>(bytes), WebSocketMessageType.Text, true, CancellationToken.None);
    }

    public void SendAttach(string sessionId)
    {
        Send(JsonSerializer.Serialize(new
        {
            type = "attach",
            data = new { session_id = sessionId, since_seq = 0 }
        }));
    }

    public void SendTermIn(string sessionId, string dataB64)
    {
        Send(JsonSerializer.Serialize(new
        {
            type = "term_in",
            session_id = sessionId,
            data_b64 = dataB64
        }));
    }

    public void SendResize(string sessionId, int cols, int rows)
    {
        Send(JsonSerializer.Serialize(new
        {
            type = "resize",
            session_id = sessionId,
            data = new { cols, rows }
        }));
    }

    public void SendAction(string sessionId, string kind)
    {
        Send(JsonSerializer.Serialize(new
        {
            type = "action",
            session_id = sessionId,
            data = new { kind }
        }));
    }

    public void SendChatIn(string sessionId, string content, List<ChatContentPart> contentParts)
    {
        var parts = contentParts.Select(p =>
        {
            var part = new Dictionary<string, object> { ["type"] = p.Type };
            if (p.Text != null) part["text"] = p.Text;
            if (p.Source != null)
                part["source"] = new Dictionary<string, object>
                {
                    ["type"] = p.Source.Type,
                    ["media_type"] = p.Source.MediaType,
                    ["data"] = p.Source.Data,
                };
            return part;
        }).ToList<object>();

        var dict = new Dictionary<string, object>
        {
            ["type"] = "chat_in",
            ["session_id"] = sessionId,
            ["data"] = new Dictionary<string, object>
            {
                ["content"] = content,
                ["content_parts"] = parts,
            },
        };
        Send(JsonSerializer.Serialize(dict));
    }
}
