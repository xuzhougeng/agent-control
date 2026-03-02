using AgentControlWin.Core;
using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using Windows.ApplicationModel.DataTransfer;

namespace AgentControlWin.Views;

public sealed partial class ChatPage : Page
{
    private AppState? _appState;

    public ChatPage()
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
        _appState.ChatMessages.CollectionChanged += ChatMessages_CollectionChanged;
        Refresh();
    }

    private void OnUnloaded(object sender, RoutedEventArgs e)
    {
        if (_appState == null) return;
        _appState.PropertyChanged -= AppState_PropertyChanged;
        _appState.ChatMessages.CollectionChanged -= ChatMessages_CollectionChanged;
    }

    private void AppState_PropertyChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        switch (e.PropertyName)
        {
            case nameof(AppState.SelectedChatSessionId):
            case nameof(AppState.SelectedSessionId):
                Refresh();
                break;
            case nameof(AppState.ChatRunState):
            case nameof(AppState.ChatRunStateText):
                UpdateRunState();
                break;
        }
    }

    private void ChatMessages_CollectionChanged(object? sender,
        System.Collections.Specialized.NotifyCollectionChangedEventArgs e)
    {
        RefreshMessages();
        ScrollToBottom();
    }

    private void Refresh()
    {
        if (_appState == null) return;
        UpdateSessionInfo();
        UpdateRunState();
        RefreshMessages();
    }

    private Session? SelectedSession =>
        _appState?.SelectedChatSessionId != null
            ? _appState.Sessions.FirstOrDefault(s => s.SessionId == _appState.SelectedChatSessionId)
            : _appState?.SelectedSessionId != null
                ? _appState.Sessions.FirstOrDefault(s => s.SessionId == _appState.SelectedSessionId)
                : null;

    private bool SelectedSessionIsChat => SelectedSession?.IsChat == true;

    private void UpdateSessionInfo()
    {
        var session = SelectedSession;
        SessionInfoText.Text = session != null
            ? $"Session: {session.ShortId}"
            : "Session: (none)";

        SwitchToChatBtn.Visibility = session != null && !session.IsChat
            ? Visibility.Visible : Visibility.Collapsed;

        InputBox.IsEnabled = SelectedSessionIsChat;
        SendBtn.IsEnabled = SelectedSessionIsChat;

        var hasSession = session != null;
        EmptyState.Visibility = hasSession && _appState?.ChatMessages.Count > 0
            ? Visibility.Collapsed : Visibility.Visible;
        if (!hasSession)
        {
            EmptyStateTitle.Text = "Select a session";
            EmptyStateSubtitle.Text = "Create or select a chat session from the sidebar.";
        }
        else if (!SelectedSessionIsChat)
        {
            EmptyStateTitle.Text = "Terminal mode";
            EmptyStateSubtitle.Text = "This session is in Terminal mode. Click \"Switch to Chat\" to continue.";
        }
    }

    private void UpdateRunState()
    {
        if (_appState == null) return;
        var state = _appState.ChatRunState;
        var text = _appState.ChatRunStateText;

        if (state == ChatRunState.Idle || string.IsNullOrEmpty(text))
        {
            RunStateBar.Visibility = Visibility.Collapsed;
            return;
        }

        RunStateBar.Visibility = Visibility.Visible;
        RunStateText.Text = text;
        RunStateBar.Background = state switch
        {
            ChatRunState.Running => new SolidColorBrush(Windows.UI.Color.FromArgb(0x1A, 0x31, 0x5f, 0x72)),
            ChatRunState.Slow => new SolidColorBrush(Windows.UI.Color.FromArgb(0x1A, 0xc8, 0x75, 0x33)),
            ChatRunState.Error => new SolidColorBrush(Windows.UI.Color.FromArgb(0x1A, 0xd9, 0x4f, 0x4f)),
            _ => new SolidColorBrush(Colors.Transparent),
        };
        RunStateText.Foreground = state switch
        {
            ChatRunState.Running => new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0x31, 0x5f, 0x72)),
            ChatRunState.Slow => new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0xc8, 0x75, 0x33)),
            ChatRunState.Error => new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0xd9, 0x4f, 0x4f)),
            _ => new SolidColorBrush(Colors.Gray),
        };
    }

    private void RefreshMessages()
    {
        if (_appState == null) return;
        var msgs = _appState.ChatMessages.ToList();

        MessagesPanel.Children.Clear();
        MessagesPanel.Children.Add(EmptyState);
        foreach (var bubble in msgs.Select(CreateBubble))
            MessagesPanel.Children.Add(bubble);

        var hasSession = SelectedSession != null;
        EmptyState.Visibility = hasSession && msgs.Count > 0
            ? Visibility.Collapsed : Visibility.Visible;
    }

    private static UIElement CreateBubble(ChatMessage msg)
    {
        var isUser = msg.IsUser;
        var border = new Border
        {
            CornerRadius = new CornerRadius(14),
            Padding = new Thickness(12, 9, 12, 9),
            Margin = new Thickness(isUser ? 80 : 0, 0, isUser ? 0 : 80, 10),
            HorizontalAlignment = isUser ? HorizontalAlignment.Right : HorizontalAlignment.Left,
            MaxWidth = 640,
        };

        if (isUser)
        {
            border.Background = new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0x31, 0x5f, 0x72));
            border.BorderBrush = new SolidColorBrush(Windows.UI.Color.FromArgb(0x14, 0xFF, 0xFF, 0xFF));
            border.BorderThickness = new Thickness(1);
        }
        else
        {
            border.Background = new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0xf5, 0xed, 0xe0));
            border.BorderBrush = new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0xd0, 0xc8, 0xbe));
            border.BorderThickness = new Thickness(1);
        }

        var text = GetDisplayText(msg);
        var tb = new TextBlock
        {
            Text = text,
            FontSize = 13,
            Foreground = isUser
                ? new SolidColorBrush(Colors.White)
                : new SolidColorBrush(Windows.UI.Color.FromArgb(0xFF, 0x1d, 0x24, 0x30)),
            TextWrapping = TextWrapping.Wrap,
        };

        // Add operations if present
        if (msg.Meta?.Operations?.Count > 0)
        {
            var stack = new StackPanel { Spacing = 3 };
            stack.Children.Add(tb);
            var opHeader = new TextBlock
            {
                Text = "INTERMEDIATE STEPS",
                FontSize = 9,
                Foreground = new SolidColorBrush(isUser
                    ? Windows.UI.Color.FromArgb(0xB8, 0xFF, 0xFF, 0xFF)
                    : Windows.UI.Color.FromArgb(0xFF, 0x59, 0x61, 0x6c)),
                Margin = new Thickness(0, 6, 0, 2),
            };
            stack.Children.Add(opHeader);
            for (int i = 0; i < msg.Meta.Operations.Count; i++)
            {
                stack.Children.Add(new TextBlock
                {
                    Text = $"{i + 1}. {msg.Meta.Operations[i]}",
                    FontSize = 11,
                    Foreground = new SolidColorBrush(isUser
                        ? Windows.UI.Color.FromArgb(0xCC, 0xFF, 0xFF, 0xFF)
                        : Windows.UI.Color.FromArgb(0xFF, 0x59, 0x61, 0x6c)),
                    TextWrapping = TextWrapping.Wrap,
                });
            }
            border.Child = stack;
        }
        else
        {
            border.Child = tb;
        }

        return border;
    }

    private static string GetDisplayText(ChatMessage msg)
    {
        if (msg.Meta?.ContentParts?.Count > 0)
        {
            var texts = msg.Meta.ContentParts
                .Where(p => p.Type == "text" && !string.IsNullOrWhiteSpace(p.Text))
                .Select(p => p.Text!.Trim())
                .ToList();
            if (texts.Count > 0) return string.Join("\n\n", texts);
        }
        return msg.Content;
    }

    private void ScrollToBottom()
    {
        MessagesScrollViewer.UpdateLayout();
        MessagesScrollViewer.ChangeView(null, MessagesScrollViewer.ScrollableHeight, null);
    }

    private void InputBox_KeyDown(object sender, KeyRoutedEventArgs e)
    {
        if (e.Key == Windows.System.VirtualKey.Enter)
        {
            var isShift = Microsoft.UI.Input.InputKeyboardSource
                .GetKeyStateForCurrentThread(Windows.System.VirtualKey.Shift)
                .HasFlag(Windows.UI.Core.CoreVirtualKeyStates.Down);
            if (!isShift)
            {
                e.Handled = true;
                Send();
            }
        }
    }

    private void Send_Click(object sender, RoutedEventArgs e) => Send();

    private void Send()
    {
        if (_appState == null || !SelectedSessionIsChat) return;
        var text = InputBox.Text?.Trim() ?? "";
        if (string.IsNullOrEmpty(text)) return;
        _appState.SendChatMessage(text, []);
        InputBox.Text = "";
    }

    private void CopySessionId_Click(object sender, RoutedEventArgs e)
    {
        var sid = _appState?.SelectedChatSessionId ?? _appState?.SelectedSessionId;
        if (string.IsNullOrEmpty(sid)) return;
        var dp = new DataPackage();
        dp.SetText(sid);
        Clipboard.SetContent(dp);
        CopySessionIdBtn.Content = "Copied!";
        DispatcherQueue.TryEnqueue(async () =>
        {
            await Task.Delay(1400);
            CopySessionIdBtn.Content = "Copy ID";
        });
    }

    private void SwitchToChat_Click(object sender, RoutedEventArgs e)
    {
        var sid = _appState?.SelectedSessionId;
        if (!string.IsNullOrEmpty(sid))
            _ = _appState?.SwitchSessionToChatAsync(sid);
    }
}
