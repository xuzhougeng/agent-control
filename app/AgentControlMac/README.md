# Agent Control — Native Clients (macOS + iOS)

SwiftUI apps that connect to a running `cc-control` server (REST + WebSocket) and provide a native terminal-based UI for managing Claude Code sessions.

## Prerequisites

- macOS 13+ / iOS 16+
- Xcode 15+
- [XcodeGen](https://github.com/yonaskolb/XcodeGen) (for generating the Xcode project)

## Setup

```bash
brew install xcodegen

cd app/AgentControlMac
xcodegen

open AgentControl.xcodeproj
```

Select the **AgentControlMac** scheme (macOS) or **AgentControliOS** scheme (iOS) and build with **Cmd+R**.

## Configuration

On first launch both apps connect to `http://127.0.0.1:18080` with token `admin-dev-token` (legacy single-tenant mode).

**macOS**: Change in **Settings** (Cmd+,).
**iOS**: Tap the **gear icon** in the toolbar.

| Field    | Description                                   |
|----------|-----------------------------------------------|
| Base URL | HTTP(S) address of the `cc-control` server    |
| UI Token | Bearer token created via Tenant API           |

The token is stored in the platform Keychain; the URL in UserDefaults.

### Getting a UI Token (Tenant API)

If `cc-control` is running with `-admin-token`, create a tenant token first, then generate UI/Agent tokens:

```bash
# Admin: create tenant token
curl -X POST http://127.0.0.1:18080/admin/tokens \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"type":"tenant"}'

# Tenant: create UI + Agent tokens
curl -X POST http://127.0.0.1:18080/tenant/tokens \
  -H "Authorization: Bearer <TENANT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"role":"owner"}'
```

The UI token is returned in the `ui.token` field.

For legacy compatibility, you can still create UI tokens directly via Admin API:

```bash
curl -X POST http://127.0.0.1:18080/admin/tokens \
  -H "Authorization: Bearer <ADMIN_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"type":"ui","role":"owner"}'
```

### Connecting to a Remote Server

For a centrally deployed `cc-control` (see [docs/deploy-public-server.md](../../docs/deploy-public-server.md)):

| Deployment         | Base URL example              | Notes |
|--------------------|-------------------------------|-------|
| 方案 A (直连 HTTP) | `http://1.2.3.4:18080`        | Works; token is sent in cleartext |
| 方案 B (Nginx+TLS) | `https://your-domain.com`     | Preferred; encrypts token and terminal traffic |
| 方案 B' (自签名)   | `https://1.2.3.4`             | Enable **Skip TLS verification** in Settings |

## Features

- Server list with online/offline status
- Session list filtered by selected server
- Create / resume sessions
- Full terminal emulation (SwiftTerm) with input, output, and resize
- Pending approval queue with one-click Approve / Reject
- WebSocket auto-reconnect with connection status indicator
- **iOS**: Quick-input keybar (Esc, Tab, Ctrl-C, arrows) above terminal
- **iOS**: Background/foreground lifecycle (auto disconnect/reconnect WS)

## Architecture

```
Sources/
  Shared/                         ← both targets
    Core/
      APIClient.swift             — REST via URLSession async/await
      WSClient.swift              — WebSocket with auto-reconnect
      AppState.swift              — central ObservableObject
      Models.swift                — Codable REST/WS models
      KeychainHelper.swift        — platform-agnostic Keychain wrapper
      TLSBypassDelegate.swift     — self-signed cert bypass
    TerminalBridge.swift          — feeds data to SwiftTerm TerminalView
    Views/
      ContentView.swift           — NavigationSplitView (sidebar + detail)
      SidebarView.swift           — server & session list
      ApprovalPanelView.swift     — pending approvals
      NewSessionSheet.swift       — create session form
  macOS/
    App/AgentControlMacApp.swift  — @main + Settings scene
    Views/
      TerminalContainerView.swift — NSViewRepresentable
      SettingsView.swift          — macOS Settings panel
  iOS/
    App/AgentControliOSApp.swift  — @main + scenePhase lifecycle
    Views/
      TerminalContainerView.swift — UIViewRepresentable + keybar
      SettingsView.swift          — iOS NavigationStack form
```
