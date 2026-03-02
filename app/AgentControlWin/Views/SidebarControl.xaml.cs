using AgentControlWin.Core;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using System.Collections.ObjectModel;

namespace AgentControlWin.Views;

public sealed partial class SidebarControl : UserControl
{
    private AppState? _appState;
    public event Action<string>? OnNavigate;

    public AppState? AppState
    {
        get => _appState;
        set
        {
            if (_appState != null)
                _appState.PropertyChanged -= AppState_PropertyChanged;
            _appState = value;
            if (_appState != null)
            {
                _appState.PropertyChanged += AppState_PropertyChanged;
                _appState.Servers.CollectionChanged += (_, _) => RefreshServers();
                _appState.Sessions.CollectionChanged += (_, _) => RefreshSessions();
                RefreshServers();
                RefreshSessions();
            }
        }
    }

    public SidebarControl()
    {
        this.InitializeComponent();
    }

    private void AppState_PropertyChanged(object? sender, System.ComponentModel.PropertyChangedEventArgs e)
    {
        switch (e.PropertyName)
        {
            case nameof(AppState.SelectedServerId):
                HighlightSelectedServer();
                break;
            case nameof(AppState.SelectedSessionId):
            case nameof(AppState.SelectedChatSessionId):
                RefreshSessions();
                break;
            case nameof(AppState.PendingApprovals):
                RefreshApprovals();
                break;
        }
    }

    private void RefreshServers()
    {
        if (_appState == null) return;
        ServerCountBadge.Text = _appState.Servers.Count.ToString();
        ServersList.ItemsSource = _appState.Servers.ToList();
        NoServersText.Visibility = _appState.Servers.Count == 0 ? Visibility.Visible : Visibility.Collapsed;
        HighlightSelectedServer();
    }

    private void HighlightSelectedServer()
    {
        // Highlighting is handled via x:Bind Tag comparison in code-behind for simplicity
    }

    private void RefreshSessions()
    {
        if (_appState == null) return;
        var sessions = _appState.Sessions.ToList();
        SessionCountBadge.Text = sessions.Count.ToString();

        var running = sessions.Where(s => s.IsRunning).ToList();
        var stopped = sessions.Where(s => !s.IsRunning).ToList();

        RunningSessionsList.ItemsSource = running;
        StoppedSessionsList.ItemsSource = stopped;

        SessionsRunningGroup.Visibility = running.Count > 0 ? Visibility.Visible : Visibility.Collapsed;
        SessionsStoppedGroup.Visibility = stopped.Count > 0 ? Visibility.Visible : Visibility.Collapsed;
        NoSessionsText.Visibility = sessions.Count == 0 ? Visibility.Visible : Visibility.Collapsed;
        RefreshApprovals();
    }

    private void RefreshApprovals()
    {
        if (_appState == null) return;
        var pending = _appState.PendingApprovals.ToList();
        ApprovalCountBadge.Text = pending.Count.ToString();
        ApprovalsList.ItemsSource = pending;
        ApprovalsBorder.Visibility = pending.Count > 0 ? Visibility.Visible : Visibility.Collapsed;
    }

    private void RefreshServers_Click(object sender, RoutedEventArgs e)
        => _ = _appState?.FetchServersAsync();

    private void RefreshSessions_Click(object sender, RoutedEventArgs e)
        => _ = _appState?.FetchSessionsAsync();

    private void ServerItem_Click(object sender, RoutedEventArgs e)
    {
        if (sender is Button btn && btn.Tag is string serverId && _appState != null)
        {
            _appState.SelectedServerId = serverId;
            _ = _appState.FetchSessionsAsync();
        }
    }

    private async void NewSession_Click(object sender, RoutedEventArgs e)
    {
        if (_appState == null) return;
        var dialog = new NewSessionDialog(_appState)
        {
            XamlRoot = this.XamlRoot
        };
        await dialog.ShowAsync();
    }

    private void SessionRow_Click(object sender, Session session)
    {
        if (_appState == null) return;
        if (_appState.SelectedPage == AppDetailPage.Terminal)
        {
            if (session.IsPty)
            {
                _appState.AttachSession(session.SessionId);
                OnNavigate?.Invoke("Terminal");
            }
            else
            {
                _ = Task.Run(async () => await _appState.SwitchSessionToPtyAsync(session.SessionId))
                    .ContinueWith(_ => DispatcherQueue.TryEnqueue(() => OnNavigate?.Invoke("Terminal")));
            }
        }
        else
        {
            if (session.IsChat)
                _ = _appState.AttachChatSessionAsync(session.SessionId);
            else
            {
                _appState.SelectedSessionId = session.SessionId;
                _appState.SelectedPage = AppDetailPage.Chat;
            }
            OnNavigate?.Invoke("Chat");
        }
    }

    private void SessionRow_Stop(object sender, Session session)
        => _ = _appState?.StopSessionAsync(session.SessionId);

    private void SessionRow_Delete(object sender, Session session)
        => _ = _appState?.DeleteSessionAsync(session.SessionId);

    private void ApproveButton_Click(object sender, RoutedEventArgs e)
    {
        if (sender is Button btn && btn.Tag is string eventId && _appState != null)
        {
            var ev = _appState.Approvals.GetValueOrDefault(eventId);
            if (ev != null)
            {
                _appState.AttachSession(ev.SessionId);
                _appState.SendAction(ev.SessionId, "approve");
            }
        }
    }

    private void RejectButton_Click(object sender, RoutedEventArgs e)
    {
        if (sender is Button btn && btn.Tag is string eventId && _appState != null)
        {
            var ev = _appState.Approvals.GetValueOrDefault(eventId);
            if (ev != null)
            {
                _appState.AttachSession(ev.SessionId);
                _appState.SendAction(ev.SessionId, "reject");
            }
        }
    }
}
