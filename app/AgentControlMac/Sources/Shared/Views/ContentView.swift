import SwiftUI

struct ContentView: View {
    @EnvironmentObject var appState: AppState
    #if os(iOS)
    @State private var showSettings = false
    #endif

    var body: some View {
        NavigationSplitView {
            SidebarView()
        } detail: {
            VStack(spacing: 0) {
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
