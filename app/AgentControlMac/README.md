# Agent Control — macOS Native Client

A SwiftUI macOS app that connects to a running `cc-control` server (REST + WebSocket) and provides a native terminal-based UI for managing Claude Code sessions.

## Prerequisites

- macOS 13 (Ventura) or later
- Xcode 15+
- [XcodeGen](https://github.com/yonaskolb/XcodeGen) (for generating the Xcode project)

## Setup

```bash
# Install XcodeGen if you don't have it
brew install xcodegen

# Generate the Xcode project
cd macos/AgentControlMac
xcodegen

# Open in Xcode
open AgentControlMac.xcodeproj
```

Build and run with **Cmd+R**.

## Configuration

On first launch the app connects to `http://127.0.0.1:18080` with token `admin-dev-token`.

Change these in **Settings** (Cmd+,):

| Field    | Description                                   |
|----------|-----------------------------------------------|
| Base URL | HTTP(S) address of the `cc-control` server    |
| UI Token | Bearer token matching `cc-control -ui-token`  |

The token is stored in the macOS Keychain; the URL in UserDefaults.

### Connecting to a Remote Server

For a centrally deployed `cc-control` (see [docs/deploy-public-server.md](../docs/deploy-public-server.md)):

| Deployment         | Base URL example              | Notes |
|--------------------|-------------------------------|-------|
| 方案 A (直连 HTTP) | `http://1.2.3.4:18080`        | Works; token is sent in cleartext |
| 方案 B (Nginx+TLS) | `https://your-domain.com`     | Preferred; encrypts token and terminal traffic |
| 方案 B' (自签名)   | `https://1.2.3.4`             | Enable **Skip TLS verification** in Settings |

Use the **same UI_TOKEN** that was set when starting `cc-control` (`-ui-token`). No browser or web UI needed; the native app talks to the server directly via REST + WebSocket.

## Features

- Server list with online/offline status
- Session list filtered by selected server
- Create / resume sessions
- Full terminal emulation (SwiftTerm) with input, output, and resize
- Pending approval queue with one-click Approve / Reject
- WebSocket auto-reconnect with connection status indicator

## Architecture

```
App (SwiftUI)
 ├── APIClient   — REST calls via URLSession async/await
 ├── WSClient    — URLSessionWebSocketTask with auto-reconnect
 ├── AppState    — central ObservableObject coordinating everything
 └── Views
      ├── NavigationSplitView (sidebar + detail)
      ├── TerminalContainerView (SwiftTerm NSViewRepresentable)
      ├── ApprovalPanelView
      ├── NewSessionSheet
      └── SettingsView (macOS Settings scene)
```
