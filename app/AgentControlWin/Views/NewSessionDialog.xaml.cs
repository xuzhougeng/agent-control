using AgentControlWin.Core;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace AgentControlWin.Views;

public sealed partial class NewSessionDialog : ContentDialog
{
    private readonly AppState _appState;

    public NewSessionDialog(AppState appState)
    {
        _appState = appState;
        this.InitializeComponent();

        // Pre-fill server info
        ServerIdText.Text = appState.SelectedServerId ?? "(none)";

        // Show allowed roots if available
        var server = appState.Servers.FirstOrDefault(s => s.ServerId == appState.SelectedServerId);
        if (server?.AllowRoots?.Count > 0)
        {
            AllowedRootsPanel.Visibility = Visibility.Visible;
            AllowedRootsItems.ItemsSource = server.AllowRoots;
        }

        this.PrimaryButtonClick += OnCreateClick;
    }

    private async void OnCreateClick(ContentDialog sender, ContentDialogButtonClickEventArgs args)
    {
        var deferral = args.GetDeferral();
        try
        {
            var cwd = CwdBox.Text.Trim();
            if (string.IsNullOrEmpty(cwd))
            {
                ErrorText.Text = "Working directory is required.";
                ErrorText.Visibility = Visibility.Visible;
                args.Cancel = true;
                return;
            }

            ErrorText.Visibility = Visibility.Collapsed;
            var sessionId = SessionIdBox.Text.Trim();
            var env = ParseEnv(EnvBox.Text);

            // Determine session type
            var isChat = SessionTypeCombo.SelectedItem is ComboBoxItem cbi
                && cbi.Tag as string == "chat";

            if (isChat)
                await _appState.CreateChatSessionAsync(cwd, sessionId.Length > 0 ? sessionId : null, env);
            else
                await _appState.CreateSessionAsync(cwd, sessionId.Length > 0 ? sessionId : null, env);
        }
        catch (Exception ex)
        {
            ErrorText.Text = ex.Message;
            ErrorText.Visibility = Visibility.Visible;
            args.Cancel = true;
        }
        finally
        {
            deferral.Complete();
        }
    }

    private void CwdBox_TextChanged(object sender, TextChangedEventArgs e)
    {
        IsPrimaryButtonEnabled = !string.IsNullOrWhiteSpace(CwdBox.Text)
            && _appState.SelectedServerId != null;
    }

    private void AllowedRoot_Click(object sender, RoutedEventArgs e)
    {
        if (sender is Button btn && btn.Tag is string root)
            CwdBox.Text = root;
    }

    private static Dictionary<string, string> ParseEnv(string input)
    {
        var env = new Dictionary<string, string>();
        foreach (var pair in input.Split(','))
        {
            var t = pair.Trim();
            var idx = t.IndexOf('=');
            if (idx > 0)
                env[t[..idx]] = t[(idx + 1)..];
        }
        return env;
    }
}
