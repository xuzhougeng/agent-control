using AgentControlWin.Core;
using Microsoft.UI;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;
using Windows.Graphics;

namespace AgentControlWin.Views;

public sealed partial class MainWindow : Window
{
    public static AppState AppState { get; private set; } = null!;

    public MainWindow()
    {
        this.InitializeComponent();

        // Set window size
        var appWindow = GetAppWindow();
        appWindow.Resize(new SizeInt32(1200, 800));
        appWindow.Title = "Agent Control";

        // Initialize AppState
        AppState = new AppState(DispatcherQueue);
        AppState.PropertyChanged += OnAppStateChanged;

        // Wire sidebar
        Sidebar.AppState = AppState;
        Sidebar.OnNavigate += NavigateTo;

        // Start services
        AppState.Start();

        // Default navigation to Terminal
        NavigateTo("Terminal");
    }

    private AppWindow GetAppWindow()
    {
        var hWnd = WinRT.Interop.WindowNative.GetWindowHandle(this);
        var windowId = Win32Interop.GetWindowIdFromWindow(hWnd);
        return AppWindow.GetFromWindowId(windowId);
    }

    private void NavigateTo(string page)
    {
        switch (page)
        {
            case "Terminal":
                AppState.SelectedPage = AppDetailPage.Terminal;
                ContentFrame.Navigate(typeof(TerminalPage));
                UpdateNavButtonHighlight(isTerminal: true);
                break;
            case "Chat":
                AppState.SelectedPage = AppDetailPage.Chat;
                ContentFrame.Navigate(typeof(ChatPage));
                UpdateNavButtonHighlight(isTerminal: false);
                break;
            case "Settings":
                ContentFrame.Navigate(typeof(SettingsPage));
                break;
        }
    }

    private void UpdateNavButtonHighlight(bool isTerminal)
    {
        var accentBrush = new SolidColorBrush(Color.FromArgb(0xFF, 0x31, 0x5f, 0x72));
        var mutedBrush = new SolidColorBrush(Color.FromArgb(0xFF, 0x59, 0x61, 0x6c));
        var surfaceBrush = new SolidColorBrush(Color.FromArgb(0xFF, 0xf5, 0xed, 0xe0));
        var whiteBrush = new SolidColorBrush(Colors.White);
        var borderBrush = new SolidColorBrush(Color.FromArgb(0xFF, 0xd0, 0xc8, 0xbe));

        TerminalNavBtn.Background = isTerminal ? accentBrush : surfaceBrush;
        TerminalNavBtn.Foreground = isTerminal ? whiteBrush : mutedBrush;
        TerminalNavBtn.BorderBrush = isTerminal ? accentBrush : borderBrush;

        ChatNavBtn.Background = isTerminal ? surfaceBrush : accentBrush;
        ChatNavBtn.Foreground = isTerminal ? mutedBrush : whiteBrush;
        ChatNavBtn.BorderBrush = isTerminal ? borderBrush : accentBrush;
    }

    private void OnAppStateChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName == nameof(AppState.WsConnected))
        {
            WsIndicator.Fill = AppState.WsConnected
                ? new SolidColorBrush(Color.FromArgb(0xFF, 0x3a, 0x9e, 0x74))
                : new SolidColorBrush(Color.FromArgb(0xFF, 0xd9, 0x4f, 0x4f));
            ToolTipService.SetToolTip(WsIndicator,
                AppState.WsConnected ? "WebSocket Connected" : "WebSocket Disconnected");
        }
    }

    private void TerminalNav_Click(object sender, RoutedEventArgs e) => NavigateTo("Terminal");
    private void ChatNav_Click(object sender, RoutedEventArgs e) => NavigateTo("Chat");
    private void SettingsBtn_Click(object sender, RoutedEventArgs e) => NavigateTo("Settings");
}
