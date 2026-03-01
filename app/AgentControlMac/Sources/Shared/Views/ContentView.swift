import SwiftUI

#if os(iOS)
enum AppTab: Hashable, CaseIterable {
    case home, terminal, chat, settings

    var index: Int {
        switch self {
        case .home: return 0
        case .terminal: return 1
        case .chat: return 2
        case .settings: return 3
        }
    }

    static func from(index: Int) -> AppTab {
        switch index {
        case 0: return .home
        case 1: return .terminal
        case 2: return .chat
        default: return .settings
        }
    }
}
#endif

struct ContentView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        ZStack {
            WorkspaceRootBackground()
                .ignoresSafeArea()
            #if os(macOS)
            macOSBody
            #else
            iOSBody
            #endif
        }
    }

    @ViewBuilder
    private var detailBody: some View {
        Group {
            if appState.selectedPage == .chat {
                ChatDetailView()
                    .padding(14)
            } else {
                VStack(spacing: 10) {
                    if let hint = appState.connectionHint {
                        ConnectionHintBanner(message: hint)
                    }
                    TerminalContainerView()
                }
                .padding(14)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
    }

    #if os(macOS)
    private var macOSBody: some View {
        NavigationSplitView {
            SidebarView()
        } detail: {
            detailBody
        }
        .navigationSplitViewStyle(.balanced)
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
            ZStack {
                WorkspaceRootBackground()
                    .ignoresSafeArea()

                TabView(selection: $selectedTab) {
                    HomeTabView(selectedTab: $selectedTab)
                        .tag(AppTab.home)

                    TerminalTabView(selectedTab: $selectedTab)
                        .tag(AppTab.terminal)

                    ChatTabView(selectedTab: $selectedTab)
                        .tag(AppTab.chat)

                    SettingsView(showDismiss: false)
                        .tag(AppTab.settings)
                }
                .tabViewStyle(.page(indexDisplayMode: .never))
            }
            .simultaneousGesture(
                DragGesture(minimumDistance: 36)
                    .onEnded { value in
                        guard abs(value.translation.width) > abs(value.translation.height),
                              abs(value.translation.width) > 70 else { return }
                        let next = value.translation.width < 0 ? selectedTab.index + 1 : selectedTab.index - 1
                        guard (0...3).contains(next) else { return }
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
            .onChange(of: appState.selectedPage) { newValue in
                guard horizontalSizeClass == .compact else { return }
                let target: AppTab = (newValue == .chat) ? .chat : .terminal
                guard selectedTab != target else { return }
                withAnimation(.easeInOut(duration: 0.2)) {
                    selectedTab = target
                }
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
            detailBody
                .toolbar(.hidden, for: .navigationBar)
        }
        .navigationSplitViewStyle(.balanced)
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
            dot(for: .chat)
            dot(for: .settings)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 7)
        .background(
            Capsule(style: .continuous)
                .fill(WorkspaceTheme.surfaceStrong)
                .overlay(Capsule(style: .continuous).stroke(WorkspaceTheme.borderStrong, lineWidth: 1))
        )
        .shadow(color: WorkspaceTheme.shadow, radius: 12, y: 5)
    }

    private func dot(for tab: AppTab, showBadge: Bool = false) -> some View {
        Button {
            withAnimation(.easeInOut(duration: 0.2)) {
                selectedTab = tab
            }
        } label: {
            ZStack(alignment: .topTrailing) {
                Circle()
                    .fill(selectedTab == tab ? WorkspaceTheme.accent : WorkspaceTheme.textMuted.opacity(0.45))
                    .frame(width: selectedTab == tab ? 8 : 7, height: selectedTab == tab ? 8 : 7)
                if showBadge {
                    Circle()
                        .fill(WorkspaceTheme.danger)
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
        case .chat: return "Chat"
        case .settings: return "Settings"
        }
    }
}
#endif

// MARK: - Shared helper views

struct ConnectionBadge: View {
    let connected: Bool

    var body: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(connected ? WorkspaceTheme.success : WorkspaceTheme.danger)
                .frame(width: 8, height: 8)
            Text(connected ? "WS: connected" : "WS: disconnected")
                .font(.system(size: 11, weight: .semibold))
                .foregroundColor(connected ? WorkspaceTheme.success : WorkspaceTheme.danger)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 5)
        .background(
            Capsule()
                .fill(connected ? WorkspaceTheme.successSoft : WorkspaceTheme.dangerSoft)
                .overlay(Capsule().strokeBorder(connected ? WorkspaceTheme.success.opacity(0.35) : WorkspaceTheme.danger.opacity(0.35)))
        )
    }
}

struct ConnectionHintBanner: View {
    let message: String

    var body: some View {
        HStack(alignment: .center, spacing: 10) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundColor(WorkspaceTheme.warning)
            Text(message)
                .font(.caption)
                .foregroundColor(WorkspaceTheme.textMuted)
            Spacer()
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .background(
            RoundedRectangle(cornerRadius: WorkspaceTheme.cornerRadiusSmall, style: .continuous)
                .fill(WorkspaceTheme.warningSoft)
                .overlay(
                    RoundedRectangle(cornerRadius: WorkspaceTheme.cornerRadiusSmall, style: .continuous)
                        .stroke(WorkspaceTheme.warning.opacity(0.32), lineWidth: 1)
                )
        )
    }
}

// MARK: - Workspace Theme

enum WorkspaceTheme {
    static let bgBase = Color(red: 0.937, green: 0.902, blue: 0.855) // #efe6da
    static let bgBaseStrong = Color(red: 0.894, green: 0.839, blue: 0.773) // #e4d6c5
    static let surface = Color(red: 0.996, green: 0.992, blue: 0.976) // #fffdf9
    static let surfaceStrong = Color(red: 0.965, green: 0.937, blue: 0.902) // #f6efe6
    static let border = Color(red: 0.851, green: 0.808, blue: 0.749) // #d9cebf
    static let borderStrong = Color(red: 0.784, green: 0.718, blue: 0.624) // #c8b79f
    static let text = Color(red: 0.114, green: 0.141, blue: 0.188) // #1d2430
    static let textMuted = Color(red: 0.349, green: 0.380, blue: 0.424) // #59616c
    static let textSoft = Color(red: 0.541, green: 0.518, blue: 0.478) // #8a847a
    static let accent = Color(red: 0.192, green: 0.373, blue: 0.447) // #315f72
    static let accentSoft = Color(red: 0.192, green: 0.373, blue: 0.447).opacity(0.12)
    static let success = Color(red: 0.165, green: 0.478, blue: 0.294) // #2a7a4b
    static let successSoft = Color(red: 0.165, green: 0.478, blue: 0.294).opacity(0.14)
    static let warning = Color(red: 0.718, green: 0.455, blue: 0.161) // #b77429
    static let warningSoft = Color(red: 0.718, green: 0.455, blue: 0.161).opacity(0.14)
    static let danger = Color(red: 0.624, green: 0.259, blue: 0.220) // #9f4238
    static let dangerSoft = Color(red: 0.624, green: 0.259, blue: 0.220).opacity(0.12)
    static let shadow = Color.black.opacity(0.05)

    static let cornerRadius: CGFloat = 12
    static let cornerRadiusSmall: CGFloat = 8
}

struct WorkspaceRootBackground: View {
    var body: some View {
        LinearGradient(
            colors: [WorkspaceTheme.bgBase, WorkspaceTheme.bgBaseStrong],
            startPoint: .top,
            endPoint: .bottom
        )
        .overlay(
            RadialGradient(
                colors: [Color.white.opacity(0.48), .clear],
                center: .topLeading,
                startRadius: 30,
                endRadius: 420
            )
        )
    }
}

struct WorkspacePanel<Content: View>: View {
    let inset: CGFloat
    let content: Content

    init(inset: CGFloat = 12, @ViewBuilder content: () -> Content) {
        self.inset = inset
        self.content = content()
    }

    var body: some View {
        content
            .padding(inset)
            .background(
                RoundedRectangle(cornerRadius: WorkspaceTheme.cornerRadius, style: .continuous)
                    .fill(WorkspaceTheme.surface)
                    .overlay(
                        RoundedRectangle(cornerRadius: WorkspaceTheme.cornerRadius, style: .continuous)
                            .stroke(WorkspaceTheme.border.opacity(0.7), lineWidth: 1)
                    )
                    .shadow(color: WorkspaceTheme.shadow, radius: 7, y: 2)
            )
    }
}

struct WorkspaceSectionTitle: View {
    let text: String

    var body: some View {
        Text(text)
            .font(.system(size: 11, weight: .bold))
            .foregroundColor(WorkspaceTheme.textSoft)
            .tracking(1.2)
            .textCase(.uppercase)
    }
}

struct WorkspaceCountBadge: View {
    let text: String
    var color: Color = WorkspaceTheme.accent

    var body: some View {
        Text(text)
            .font(.system(size: 10, weight: .bold))
            .foregroundColor(.white)
            .padding(.horizontal, 7)
            .padding(.vertical, 3)
            .background(Capsule(style: .continuous).fill(color))
    }
}
