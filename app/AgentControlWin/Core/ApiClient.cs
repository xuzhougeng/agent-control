using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Net.Security;

namespace AgentControlWin.Core;

public class ApiException : Exception
{
    public int StatusCode { get; }
    public ApiException(int code, string message) : base($"HTTP {code}: {message}") => StatusCode = code;
}

public class ApiClient
{
    private string _baseUrl = "";
    private string _token = "";
    private bool _skipTlsVerify;
    private HttpClient? _httpClient;

    private HttpClient HttpClient
    {
        get
        {
            if (_httpClient != null) return _httpClient;
            var handler = new HttpClientHandler();
            if (_skipTlsVerify)
            {
                handler.ServerCertificateCustomValidationCallback =
                    (_, _, _, _) => true;
            }
            _httpClient = new HttpClient(handler);
            _httpClient.DefaultRequestHeaders.Authorization =
                new AuthenticationHeaderValue("Bearer", _token);
            return _httpClient;
        }
    }

    public void Configure(string baseUrl, string token, bool skipTlsVerify = false)
    {
        _baseUrl = baseUrl.TrimEnd('/');
        _token = token;
        _skipTlsVerify = skipTlsVerify;
        // Reset client to pick up new settings
        _httpClient?.Dispose();
        _httpClient = null;
    }

    public static async Task CheckConnectionAsync(string baseUrl, string token, bool skipTlsVerify)
    {
        var url = baseUrl.TrimEnd('/') + "/api/servers";
        var handler = new HttpClientHandler();
        if (skipTlsVerify)
            handler.ServerCertificateCustomValidationCallback = (_, _, _, _) => true;
        using var client = new HttpClient(handler);
        client.DefaultRequestHeaders.Authorization = new AuthenticationHeaderValue("Bearer", token);
        var resp = await client.GetAsync(url);
        if (!resp.IsSuccessStatusCode)
        {
            var body = await resp.Content.ReadAsStringAsync();
            throw new ApiException((int)resp.StatusCode, body);
        }
    }

    public async Task<List<Server>> GetServersAsync()
    {
        var data = await RequestAsync("/api/servers");
        var resp = JsonSerializer.Deserialize<ServersResponse>(data);
        return resp?.Servers ?? [];
    }

    public async Task<List<Session>> GetSessionsAsync(string? serverId)
    {
        var path = "/api/sessions";
        if (!string.IsNullOrEmpty(serverId))
            path += $"?server_id={Uri.EscapeDataString(serverId)}";
        var data = await RequestAsync(path);
        var resp = JsonSerializer.Deserialize<SessionsResponse>(data);
        return resp?.Sessions ?? [];
    }

    public async Task<Session> CreateSessionAsync(
        string serverId, string cwd, string? sessionId,
        Dictionary<string, string> env, SessionType sessionType,
        int cols = 120, int rows = 30)
    {
        var body = new Dictionary<string, object>
        {
            ["server_id"] = serverId,
            ["session_type"] = sessionType == SessionType.Chat ? "chat" : "pty",
            ["cwd"] = cwd,
            ["env"] = env,
        };
        if (sessionType == SessionType.Pty)
        {
            body["cols"] = cols;
            body["rows"] = rows;
        }
        if (!string.IsNullOrEmpty(sessionId))
            body["session_id"] = sessionId!.ToLower();
        var json = JsonSerializer.Serialize(body);
        var data = await RequestAsync("/api/sessions", "POST", json);
        return JsonSerializer.Deserialize<Session>(data) ?? throw new Exception("Invalid session response");
    }

    public async Task StopSessionAsync(string sessionId, int graceMs = 4000, int killAfterMs = 9000)
    {
        var body = JsonSerializer.Serialize(new { grace_ms = graceMs, kill_after_ms = killAfterMs });
        await RequestAsync($"/api/sessions/{sessionId}/stop", "POST", body);
    }

    public async Task DeleteSessionAsync(string sessionId)
    {
        await RequestAsync($"/api/sessions/{sessionId}", "DELETE");
    }

    public async Task<List<ChatMessage>> GetChatHistoryAsync(string sessionId)
    {
        var data = await RequestAsync($"/api/sessions/{sessionId}/chat");
        var resp = JsonSerializer.Deserialize<ChatHistoryResponse>(data);
        return resp?.Messages ?? [];
    }

    public async Task<Session> SwitchSessionAsync(
        string sessionId, SessionType sessionType,
        Dictionary<string, string>? env = null,
        int cols = 120, int rows = 30)
    {
        var body = new Dictionary<string, object>
        {
            ["session_type"] = sessionType == SessionType.Chat ? "chat" : "pty",
            ["env"] = env ?? [],
        };
        if (sessionType == SessionType.Pty)
        {
            body["cols"] = cols;
            body["rows"] = rows;
        }
        var json = JsonSerializer.Serialize(body);
        var data = await RequestAsync($"/api/sessions/{sessionId}/switch", "POST", json);
        return JsonSerializer.Deserialize<Session>(data) ?? throw new Exception("Invalid session response");
    }

    private async Task<string> RequestAsync(string path, string method = "GET", string? jsonBody = null)
    {
        var url = _baseUrl + path;
        using var request = new HttpRequestMessage(new HttpMethod(method), url);
        if (jsonBody != null)
        {
            request.Content = new StringContent(jsonBody, Encoding.UTF8, "application/json");
        }
        var response = await HttpClient.SendAsync(request);
        var body = await response.Content.ReadAsStringAsync();
        if (!response.IsSuccessStatusCode)
            throw new ApiException((int)response.StatusCode, body);
        return body;
    }
}
