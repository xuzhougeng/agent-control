import SwiftUI

#if os(iOS)
enum AppTab: Hashable, CaseIterable {
    case home, terminal, settings

    var index: Int {
        switch self {
        case .home: return 0
        case .terminal: return 1
        case .settings: return 2
        }
    }

    static func from(index: Int) -> AppTab {
        switch index {
        case 0: return .home
        case 1: return .terminal
        default: return .settings
        }
    }
}
#endif

struct ContentView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        #if os(macOS)
        macOSBody
        #else
        iOSBody
        #endif
    }

    // MARK: - macOS (unchanged)

    #if os(macOS)
    private var macOSBody: some View {
        NavigationSplitView {
            SidebarView()
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
            ToolbarItem(placement: .automatic) {
                if #available(macOS 14, *) {
                    SettingsLink {
                        Image(systemName: "gear")
                    }
                    .help("Settings (⌘,)")
                }
            }
        }
        .sheet(isPresented: $appState.showNewSessionSheet) {
            NewSessionSheet()
                .environmentObject(appState)
        }
    }
    #endif

    // MARK: - iOS (hybrid: compact → TabView, regular → SplitView)

    #if os(iOS)
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @State private var selectedTab: AppTab = .home
    @State private var showSettings = false
    @State private var isKeyboardVisible = false

    @ViewBuilder
    private var iOSBody: some View {
        if horizontalSizeClass == .compact {
            compactTabBody
        } else {
            regularSplitBody
        }
    }

    // MARK: iPhone (compact) — TabView

    private var compactTabBody: some View {
        NavigationStack {
            TabView(selection: $selectedTab) {
                HomeTabView(selectedTab: $selectedTab)
                    .tag(AppTab.home)

                TerminalTabView(selectedTab: $selectedTab)
                    .tag(AppTab.terminal)

                SettingsView(showDismiss: false)
                    .tag(AppTab.settings)
            }
            .tabViewStyle(.page(indexDisplayMode: .never))
            .simultaneousGesture(
                DragGesture(minimumDistance: 36)
                    .onEnded { value in
                        guard abs(value.translation.width) > abs(value.translation.height),
                              abs(value.translation.width) > 70 else { return }
                        let next = value.translation.width < 0 ? selectedTab.index + 1 : selectedTab.index - 1
                        guard (0...2).contains(next) else { return }
                        withAnimation(.easeInOut(duration: 0.2)) {
                            selectedTab = .from(index: next)
                        }
                    }
            )
            .safeAreaInset(edge: .bottom) {
                if !isKeyboardVisible {
                    CompactPageDots(selectedTab: $selectedTab, pendingApprovalCount: appState.pendingApprovals.count)
                        .padding(.bottom, 4)
                }
            }
            .onChange(of: selectedTab) { newValue in
                guard newValue != .terminal else { return }
                dismissKeyboardAndTerminalFocus()
            }
            .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardWillShowNotification)) { _ in
                isKeyboardVisible = true
            }
            .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardWillHideNotification)) { _ in
                isKeyboardVisible = false
            }
            .sheet(isPresented: $appState.showNewSessionSheet) {
                NewSessionSheet(onCreated: { selectedTab = .terminal })
                    .environmentObject(appState)
            }
        }
    }

    private func dismissKeyboardAndTerminalFocus() {
        if let terminalView = appState.terminalBridge.terminalView {
            _ = terminalView.resignFirstResponder()
            terminalView.window?.endEditing(true)
        }
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
    }

    // MARK: iPad (regular) — NavigationSplitView

    private var regularSplitBody: some View {
        NavigationSplitView {
            SidebarView(onOpenSettings: { showSettings = true })
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
            ToolbarItem(placement: .navigationBarTrailing) {
                Button { showSettings = true } label: {
                    Image(systemName: "gear")
                }
            }
        }
        .sheet(isPresented: $appState.showNewSessionSheet) {
            NewSessionSheet()
                .environmentObject(appState)
        }
        .sheet(isPresented: $showSettings) {
            SettingsView()
                .environmentObject(appState)
        }
    }
    #endif
}

#if os(iOS)
private struct CompactPageDots: View {
    @Binding var selectedTab: AppTab
    let pendingApprovalCount: Int

    var body: some View {
        HStack(spacing: 10) {
            dot(for: .home)
            dot(for: .terminal, showBadge: pendingApprovalCount > 0)
            dot(for: .settings)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 7)
        .background(.ultraThinMaterial, in: Capsule())
    }

    private func dot(for tab: AppTab, showBadge: Bool = false) -> some View {
        Button {
            withAnimation(.easeInOut(duration: 0.2)) {
                selectedTab = tab
            }
        } label: {
            ZStack(alignment: .topTrailing) {
                Circle()
                    .fill(selectedTab == tab ? Color.accentColor : Color.secondary.opacity(0.45))
                    .frame(width: selectedTab == tab ? 8 : 7, height: selectedTab == tab ? 8 : 7)
                if showBadge {
                    Circle()
                        .fill(.red)
                        .frame(width: 5, height: 5)
                        .offset(x: 3, y: -2)
                }
            }
        }
        .buttonStyle(.plain)
        .accessibilityLabel(accessibilityLabel(for: tab))
    }

    private func accessibilityLabel(for tab: AppTab) -> String {
        switch tab {
        case .home: return "Home"
        case .terminal: return pendingApprovalCount > 0 ? "Terminal, has pending approvals" : "Terminal"
        case .settings: return "Settings"
        }
    }
}
#endif

// MARK: - Shared helper views

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
