using AgentControlWin.Core;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.Web.WebView2.Core;
using System.Text.Json;

namespace AgentControlWin.Views;

public sealed partial class TerminalPage : Page
{
    private AppState? _appState;
    private Microsoft.Web.WebView2.WinUI.WebView2? _webView;
    private bool _webViewReady;
    private SessionEvent? _currentApproval;

    public TerminalPage()
    {
        this.InitializeComponent();
        Loaded += OnLoaded;
        Unloaded += OnUnloaded;
    }

    private void OnLoaded(object sender, RoutedEventArgs e)
    {
        _appState = MainWindow.AppState;
        if (_appState == null) return;
        _appState.PropertyChanged += AppState_PropertyChanged;
        _appState.OnTermOut += HandleTermOut;
        InitWebView();
        UpdateNoSessionHint();
        RefreshApproval();
    }

    private void OnUnloaded(object sender, RoutedEventArgs e)
    {
        if (_appState != null)
        {
            _appState.PropertyChanged -= AppState_PropertyChanged;
            _appState.OnTermOut -= HandleTermOut;
        }
    }

    private async void InitWebView()
    {
        _webView = new Microsoft.Web.WebView2.WinUI.WebView2
        {
            HorizontalAlignment = HorizontalAlignment.Stretch,
            VerticalAlignment = VerticalAlignment.Stretch,
        };

        // Insert behind the hint text so it covers the full container
        TerminalContainer.Children.Insert(0, _webView);

        await _webView.EnsureCoreWebView2Async();
        _webView.CoreWebView2.WebMessageReceived += OnWebMessageReceived;

        // Load terminal.html
        var htmlPath = System.IO.Path.Combine(AppContext.BaseDirectory, "Assets", "terminal.html");
        if (System.IO.File.Exists(htmlPath))
            _webView.CoreWebView2.Navigate(new Uri(htmlPath).AbsoluteUri);
        else
            _webView.NavigateToString(FallbackHtml);

        _webViewReady = true;
        UpdateNoSessionHint();
    }

    private void OnWebMessageReceived(CoreWebView2 sender, CoreWebView2WebMessageReceivedEventArgs args)
    {
        try
        {
            var text = args.TryGetWebMessageAsString();
            using var doc = JsonDocument.Parse(text);
            var root = doc.RootElement;
            var type = root.GetProperty("type").GetString();
            if (type == "term_in" && _appState != null)
            {
                var b64 = root.GetProperty("data_b64").GetString() ?? "";
                _appState.SendTerminalInput(Convert.FromBase64String(b64));
            }
            else if (type == "resize" && _appState != null)
            {
                _appState.SendResize(root.GetProperty("cols").GetInt32(), root.GetProperty("rows").GetInt32());
            }
        }
        catch (Exception ex)
        {
            System.Diagnostics.Debug.WriteLine($"[terminal] webmsg: {ex.Message}");
        }
    }

    private void HandleTermOut(string sessionId, byte[] data)
    {
        if (!_webViewReady || _webView?.CoreWebView2 == null) return;
        if (data.Length == 0)
        {
            _webView.CoreWebView2.PostWebMessageAsString(JsonSerializer.Serialize(new { type = "reset" }));
            return;
        }
        _webView.CoreWebView2.PostWebMessageAsString(
            JsonSerializer.Serialize(new { type = "term_out", data_b64 = Convert.ToBase64String(data) }));
    }

    private void AppState_PropertyChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(AppState.SelectedSessionId))
        {
            UpdateNoSessionHint();
            RefreshApproval();
        }
        else if (e.PropertyName is nameof(AppState.PendingApprovals))
        {
            RefreshApproval();
        }
    }

    private void UpdateNoSessionHint()
    {
        var hasSession = !string.IsNullOrEmpty(_appState?.SelectedSessionId);
        NoSessionHint.Visibility = hasSession ? Visibility.Collapsed : Visibility.Visible;
        if (_webView != null)
            _webView.Visibility = hasSession ? Visibility.Visible : Visibility.Collapsed;
    }

    private void RefreshApproval()
    {
        if (_appState == null) { ApprovalOverlay.Visibility = Visibility.Collapsed; return; }
        _currentApproval = _appState.PendingApprovals
            .FirstOrDefault(ev => ev.SessionId == _appState.SelectedSessionId);
        ApprovalOverlay.Visibility = _currentApproval != null ? Visibility.Visible : Visibility.Collapsed;
        if (_currentApproval != null)
            ApprovalExcerptText.Text = _currentApproval.PromptExcerpt ?? "";
    }

    private void Approve_Click(object sender, RoutedEventArgs e)
    {
        if (_currentApproval != null && _appState != null)
            _appState.SendAction(_currentApproval.SessionId, "approve");
    }

    private void Reject_Click(object sender, RoutedEventArgs e)
    {
        if (_currentApproval != null && _appState != null)
            _appState.SendAction(_currentApproval.SessionId, "reject");
    }

    private const string FallbackHtml = """
        <!DOCTYPE html>
        <html><body style="background:#0b0f20;color:#e8e8e8;font-family:monospace;padding:20px;margin:0">
        <p>terminal.html not found. Place it in Assets/ folder.</p>
        </body></html>
        """;
}
