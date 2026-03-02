# AgentControlWin

WinUI 3 Windows native client for cc-control — ported from AgentControlMac (SwiftUI).

## Requirements

- Windows 10 version 1903 (19041) or later
- .NET 8 SDK
- Windows App SDK 1.6
- Visual Studio 2022 17.9+ with "Windows application development" workload

## Build

```powershell
cd app\AgentControlWin
dotnet build
dotnet run
```

Or open `AgentControlWin.csproj` in Visual Studio 2022 and press F5.

## Project Structure

```
AgentControlWin/
├── AgentControlWin.csproj       # Project file (unpackaged, .NET 8, WinUI 3)
├── app.manifest                 # DPI awareness, Windows 10/11 compatibility
├── App.xaml / App.xaml.cs      # Application entry point + theme resources
├── Assets/
│   └── terminal.html           # xterm.js terminal (loaded in WebView2)
├── Core/
│   ├── Models.cs               # Data models (Server, Session, ChatMessage, …)
│   ├── ApiClient.cs            # REST client (Bearer token, skip-TLS option)
│   ├── WsClient.cs             # WebSocket client with exponential-backoff reconnect
│   ├── AppState.cs             # Central state (INotifyPropertyChanged + ObservableCollections)
│   ├── AppSettings.cs          # File-based settings (%APPDATA%\AgentControlWin\settings.json)
│   └── CredentialHelper.cs     # DPAPI token storage (token.dat)
├── Views/
│   ├── MainWindow.xaml/.cs     # Root window (Grid sidebar + Frame content)
│   ├── SidebarControl.xaml/.cs # Server + session + approval lists
│   ├── SessionRowControl.xaml/.cs # Individual session row
│   ├── TerminalPage.xaml/.cs   # WebView2 + xterm.js terminal
│   ├── ChatPage.xaml/.cs       # Chat messages + input bar
│   ├── SettingsPage.xaml/.cs   # URL / token / TLS settings
│   └── NewSessionDialog.xaml/.cs # ContentDialog for new session creation
└── Converters/
    ├── BoolToVisibilityConverter.cs
    └── StatusToColorConverter.cs
```

## Configuration

Settings are persisted to `%APPDATA%\AgentControlWin\settings.json`:
- `baseUrl`: Control plane URL (default: `http://127.0.0.1:18080`)
- `skipTlsVerify`: Skip TLS certificate validation

The auth token is stored encrypted with DPAPI at `%APPDATA%\AgentControlWin\token.dat`.

## Features

- **Server list** — fetched from `/api/servers`, auto-selects first
- **Session list** — grouped running/stopped, supports PTY + Chat
- **Terminal** — WebView2 + xterm.js, resize relay, approval overlay
- **Chat** — message bubbles, run-state indicator, Switch-to-Chat
- **Approvals** — sidebar badge + in-terminal overlay with Approve/Reject
- **Settings** — Base URL, token, TLS toggle, connection check
- **WebSocket** — exponential backoff reconnect (1s → 30s)
- **Theme** — warm beige (#efe6da) + teal accent (#315f72) matching cc-web
