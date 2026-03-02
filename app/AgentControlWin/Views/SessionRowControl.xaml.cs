using AgentControlWin.Core;
using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace AgentControlWin.Views;

public sealed partial class SessionRowControl : UserControl
{
    public static readonly DependencyProperty SessionDataProperty =
        DependencyProperty.Register(nameof(SessionData), typeof(Session), typeof(SessionRowControl),
            new PropertyMetadata(null, OnSessionChanged));

    public Session? SessionData
    {
        get => (Session?)GetValue(SessionDataProperty);
        set => SetValue(SessionDataProperty, value);
    }

    public event EventHandler<Session>? SessionClicked;
    public event EventHandler<Session>? SessionStopClicked;
    public event EventHandler<Session>? SessionDeleteClicked;

    private static readonly SolidColorBrush AccentSoftBrush =
        new(Color.FromArgb(0x1A, 0x31, 0x5f, 0x72));
    private static readonly SolidColorBrush AccentBrush =
        new(Color.FromArgb(0xFF, 0x31, 0x5f, 0x72));
    private static readonly SolidColorBrush SurfaceStrongBrush =
        new(Color.FromArgb(0xFF, 0xf5, 0xed, 0xe0));

    public SessionRowControl()
    {
        this.InitializeComponent();
    }

    private static void OnSessionChanged(DependencyObject d, DependencyPropertyChangedEventArgs e)
    {
        if (d is SessionRowControl ctrl) ctrl.UpdateUI();
    }

    private void UpdateUI()
    {
        if (SessionData == null) return;
        StopBtn.Visibility = (SessionData.IsPty && SessionData.IsRunning)
            ? Visibility.Visible : Visibility.Collapsed;
        ApprovalTag.Visibility = SessionData.AwaitingApproval
            ? Visibility.Visible : Visibility.Collapsed;
    }

    private void MainArea_Click(object sender, RoutedEventArgs e)
    {
        if (SessionData != null) SessionClicked?.Invoke(this, SessionData);
    }

    private void Stop_Click(object sender, RoutedEventArgs e)
    {
        if (SessionData != null) SessionStopClicked?.Invoke(this, SessionData);
    }

    private void Delete_Click(object sender, RoutedEventArgs e)
    {
        if (SessionData != null) SessionDeleteClicked?.Invoke(this, SessionData);
    }
}
