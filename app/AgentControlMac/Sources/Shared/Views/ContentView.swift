import SwiftUI

struct ContentView: View {
    @EnvironmentObject var appState: AppState
    #if os(iOS)
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @State private var showSettings = false
    @State private var showSessionTerminal = false
    #endif

    var body: some View {
        NavigationSplitView {
            #if os(iOS)
            SidebarView(
                onOpenSettings: { showSettings = true },
                onOpenSessionTerminal: horizontalSizeClass == .compact ? { showSessionTerminal = true } : nil
            )
            #else
            SidebarView()
            #endif
        } detail: {
            VStack(spacing: 0) {
                if let hint = appState.connectionHint {
                    ConnectionHintBanner(message: hint)
                }
                ApprovalPanelView()
                Divider()
                TerminalContainerView()
            }
        }
        .toolbar {
            ToolbarItem(placement: .automatic) {
                ConnectionBadge(connected: appState.wsConnected)
            }
            #if os(iOS)
            ToolbarItem(placement: .navigationBarTrailing) {
                Button { showSettings = true } label: {
                    Image(systemName: "gear")
                }
            }
            #endif
        }
        .sheet(isPresented: $appState.showNewSessionSheet) {
            NewSessionSheet()
                .environmentObject(appState)
        }
        #if os(iOS)
        .sheet(isPresented: $showSettings) {
            SettingsView()
                .environmentObject(appState)
        }
        .sheet(isPresented: $showSessionTerminal) {
            SessionTerminalSheet()
                .environmentObject(appState)
        }
        #endif
    }
}

// MARK: - Connection status badge

struct ConnectionBadge: View {
    let connected: Bool

    var body: some View {
        HStack(spacing: 4) {
            Circle()
                .fill(connected ? Color.green : Color.red)
                .frame(width: 8, height: 8)
            Text(connected ? "WS: connected" : "WS: disconnected")
                .font(.system(size: 11))
                .foregroundColor(.secondary)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 3)
        .background(
            Capsule()
                .fill(connected ? Color.green.opacity(0.12) : Color.red.opacity(0.12))
                .overlay(Capsule().strokeBorder(connected ? Color.green.opacity(0.3) : Color.red.opacity(0.3)))
        )
    }
}

struct ConnectionHintBanner: View {
    let message: String

    var body: some View {
        HStack(alignment: .center, spacing: 8) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundColor(.yellow)
            Text(message)
                .font(.caption)
                .foregroundColor(.secondary)
            Spacer()
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(Color.yellow.opacity(0.08))
    }
}

#if os(iOS)
private struct SessionTerminalSheet: View {
    @EnvironmentObject var appState: AppState
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ApprovalPanelView()
                Divider()
                TerminalContainerView()
            }
            .navigationTitle(appState.selectedSessionID.map { "Session \(String($0.prefix(8)))" } ?? "Terminal")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
        }
    }
}
#endif
