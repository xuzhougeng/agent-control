using AgentControlWin.Core;
using Microsoft.UI;using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace AgentControlWin.Views;

public sealed partial class SettingsPage : Page
{
    private AppState? _appState;

    public SettingsPage()
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
        LoadCurrentConfig();
        UpdateStatus();
    }

    private void OnUnloaded(object sender, RoutedEventArgs e)
    {
        if (_appState != null)
            _appState.PropertyChanged -= AppState_PropertyChanged;
    }

    private void AppState_PropertyChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(AppState.WsConnected)
            or nameof(AppState.Servers) or nameof(AppState.Sessions))
            UpdateStatus();
    }

    private void LoadCurrentConfig()
    {
        BaseUrlBox.Text = AppSettings.GetString("baseUrl", "http://127.0.0.1:18080")!;
        TokenBox.Password = CredentialHelper.LoadToken() ?? "admin-dev-token";
        SkipTlsToggle.IsOn = AppSettings.GetBool("skipTlsVerify");
    }

    private void UpdateStatus()
    {
        if (_appState == null) return;
        var connected = _appState.WsConnected;
        WsStatusDot.Fill = connected
            ? new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0x3a, 0x9e, 0x74))
            : new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0xd9, 0x4f, 0x4f));
        WsStatusText.Text = connected ? "Connected" : "Disconnected";
        ServersCountText.Text = _appState.Servers.Count.ToString();
        SessionsCountText.Text = _appState.Sessions.Count.ToString();
    }

    private void BaseUrl_Changed(object sender, TextChangedEventArgs e)
    {
        var valid = string.IsNullOrWhiteSpace(BaseUrlBox.Text) || AppState.IsValidBaseUrl(BaseUrlBox.Text);
        BaseUrlError.Visibility = valid ? Visibility.Collapsed : Visibility.Visible;
    }

    private async void CheckConnection_Click(object sender, RoutedEventArgs e)
    {
        CheckConnectionBtn.IsEnabled = false;
        ConnectionResultText.Visibility = Visibility.Visible;
        ConnectionResultText.Text = "Checking...";
        ConnectionResultText.Foreground = new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0x59, 0x61, 0x6c));

        try
        {
            await ApiClient.CheckConnectionAsync(
                BaseUrlBox.Text.Trim(),
                TokenBox.Password.Trim(),
                SkipTlsToggle.IsOn);
            ConnectionResultText.Text = "✓ Connection successful";
            ConnectionResultText.Foreground = new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0x3a, 0x9e, 0x74));
        }
        catch (Exception ex)
        {
            ConnectionResultText.Text = $"✕ Failed: {ex.Message}";
            ConnectionResultText.Foreground = new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0xd9, 0x4f, 0x4f));
        }
        finally
        {
            CheckConnectionBtn.IsEnabled = true;
        }
    }

    private void Save_Click(object sender, RoutedEventArgs e)
    {
        var url = BaseUrlBox.Text.Trim();
        if (!AppState.IsValidBaseUrl(url))
        {
            BaseUrlError.Visibility = Visibility.Visible;
            return;
        }
        _appState?.SaveConfig(url, TokenBox.Password.Trim(), SkipTlsToggle.IsOn);
        SaveBtn.Content = "Saved ✓";
        DispatcherQueue.TryEnqueue(async () =>
        {
            await Task.Delay(2000);
            SaveBtn.Content = "Save & Reconnect";
        });
    }
}
