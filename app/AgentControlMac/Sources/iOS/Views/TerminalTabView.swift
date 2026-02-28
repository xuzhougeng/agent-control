import SwiftUI

struct TerminalTabView: View {
    @EnvironmentObject var appState: AppState
    @Binding var selectedTab: AppTab
    @State private var showSessionDrawer = false
    @State private var showApprovals = false
    @State private var confirmStop = false

    private var currentSession: Session? {
        guard let sid = appState.selectedSessionID else { return nil }
        return appState.sessions.first { $0.sessionID == sid }
    }

    private var isActive: Bool {
        selectedTab == .terminal
    }

    var body: some View {
        ZStack {
            WorkspaceRootBackground()
                .ignoresSafeArea()

            if appState.selectedSessionID != nil {
                WorkspacePanel(inset: 0) {
                    TerminalContainerView()
                }
                .padding(12)
                .ignoresSafeArea(.keyboard)
                .onAppear { appState.terminalBridge.requestScrollToBottom() }
            } else {
                noSessionView
            }

            if showSessionDrawer {
                Color.black.opacity(0.35)
                    .ignoresSafeArea()
                    .onTapGesture {
                        withAnimation(.easeInOut(duration: 0.22)) {
                            showSessionDrawer = false
                        }
                    }
                    .transition(.opacity)
            }

            HStack(spacing: 0) {
                if showSessionDrawer {
                    SessionDrawerView(
                        isOpen: $showSessionDrawer,
                        selectedTab: $selectedTab
                    )
                    .environmentObject(appState)
                    .frame(width: min(UIScreen.main.bounds.width * 0.82, 340))
                    .transition(.move(edge: .leading))
                }
                Spacer(minLength: 0)
            }
        }
        .navigationTitle(currentSession.map { "Session \($0.shortID)" } ?? "Terminal Workspace")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if isActive {
                toolbarContent
            }
        }
        .sheet(isPresented: $showApprovals) {
            ApprovalsSheet()
                .environmentObject(appState)
        }
        .alert("Stop Session?", isPresented: $confirmStop) {
            Button("Cancel", role: .cancel) {}
            Button("Stop", role: .destructive) {
                if let sid = appState.selectedSessionID {
                    Task { await appState.stopSession(sid) }
                }
            }
        } message: {
            Text("The running session will be terminated.")
        }
        .animation(.easeInOut(duration: 0.22), value: showSessionDrawer)
        .onChange(of: selectedTab) { newValue in
            guard newValue != .terminal else { return }
            dismissKeyboardAndCloseOverlays()
        }
    }

    // MARK: - Toolbar

    @ToolbarContentBuilder
    private var toolbarContent: some ToolbarContent {
        ToolbarItem(placement: .navigationBarLeading) {
            Button {
                withAnimation(.easeInOut(duration: 0.22)) {
                    showSessionDrawer.toggle()
                }
            } label: {
                Image(systemName: "list.bullet")
            }
            .accessibilityLabel("Switch Session")
        }
        ToolbarItemGroup(placement: .navigationBarTrailing) {
            if !appState.pendingApprovals.isEmpty {
                Button { showApprovals = true } label: {
                    Image(systemName: "bell.badge")
                        .foregroundColor(.yellow)
                }
                .accessibilityLabel("\(appState.pendingApprovals.count) Pending Approvals")
            }
            if currentSession?.isRunning == true {
                Button { confirmStop = true } label: {
                    Image(systemName: "stop.fill")
                        .foregroundColor(.red)
                }
                .accessibilityLabel("Stop Session")
            }
        }
    }

    // MARK: - Empty state

    private var noSessionView: some View {
        WorkspacePanel(inset: 16) {
            VStack(spacing: 16) {
                Image(systemName: "terminal")
                    .font(.system(size: 44))
                    .foregroundColor(WorkspaceTheme.textSoft)
                Text("No session selected")
                    .font(.title3.weight(.semibold))
                    .foregroundColor(WorkspaceTheme.textMuted)

                VStack(spacing: 10) {
                    if appState.sessions.contains(where: \.isRunning) {
                        Button {
                            withAnimation(.easeInOut(duration: 0.22)) {
                                showSessionDrawer = true
                            }
                        } label: {
                            Label("Choose Session", systemImage: "list.bullet")
                        }
                        .buttonStyle(.bordered)
                        .tint(WorkspaceTheme.accent)
                    }
                    Button {
                        appState.showNewSessionSheet = true
                    } label: {
                        Label("New Session", systemImage: "plus")
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(WorkspaceTheme.accent)
                    .disabled(appState.selectedServerID == nil)
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .center)
        .padding(18)
    }

    private func dismissKeyboardAndCloseOverlays() {
        showSessionDrawer = false
        showApprovals = false
        confirmStop = false
        if let terminalView = appState.terminalBridge.terminalView {
            _ = terminalView.resignFirstResponder()
            terminalView.window?.endEditing(true)
        }
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
    }
}

// MARK: - Session Drawer

struct SessionDrawerView: View {
    @EnvironmentObject var appState: AppState
    @Binding var isOpen: Bool
    @Binding var selectedTab: AppTab

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Sessions")
                    .font(.headline)
                Spacer()
                Button {
                    withAnimation(.easeInOut(duration: 0.22)) {
                        isOpen = false
                    }
                } label: {
                    Image(systemName: "xmark")
                }
                .buttonStyle(.plain)
            }
            .padding(.horizontal, 12)
            .padding(.top, 12)
            .padding(.bottom, 8)

            ScrollView {
                VStack(spacing: 8) {
                    Button {
                        appState.showNewSessionSheet = true
                        withAnimation(.easeInOut(duration: 0.22)) {
                            isOpen = false
                        }
                    } label: {
                        Label("New Session", systemImage: "plus")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(WorkspaceTheme.accent)
                    .disabled(appState.selectedServerID == nil)

                    let running = appState.sessions.filter(\.isRunning)
                    let stopped = appState.sessions.filter { !$0.isRunning }

                    if !running.isEmpty {
                        groupTitle("Running")
                        ForEach(running) { session in
                            sessionButton(session)
                        }
                    }
                    if !stopped.isEmpty {
                        groupTitle("Stopped")
                        ForEach(stopped) { session in
                            sessionButton(session)
                        }
                    }
                    if appState.sessions.isEmpty {
                        Text("No sessions available")
                            .foregroundColor(WorkspaceTheme.textMuted)
                            .frame(maxWidth: .infinity, alignment: .center)
                            .padding(.top, 8)
                    }
                }
                .padding(.horizontal, 12)
                .padding(.bottom, 10)
            }
        }
        .background(WorkspaceTheme.surface)
    }

    private func sessionButton(_ session: Session) -> some View {
        Button {
            appState.openSession(session)
            selectedTab = session.isChat ? .chat : .terminal
            withAnimation(.easeInOut(duration: 0.22)) {
                isOpen = false
            }
        } label: {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(session.shortID)
                        .font(.system(.body, design: .monospaced))
                    Text(session.cwd)
                        .font(.caption)
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                }
                Spacer()
                if isSelected(session) {
                    Image(systemName: "checkmark")
                        .foregroundColor(WorkspaceTheme.accent)
                }
                StatusBadge(label: session.status, isOnline: session.isRunning)
            }
        }
        .foregroundColor(WorkspaceTheme.text)
    }

    private func groupTitle(_ title: String) -> some View {
        WorkspaceSectionTitle(text: title)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.top, 6)
    }

    private func isSelected(_ session: Session) -> Bool {
        if session.isChat {
            return session.sessionID == appState.selectedChatSessionID
        }
        return session.sessionID == appState.selectedSessionID
    }
}

// MARK: - Approvals Sheet

struct ApprovalsSheet: View {
    @EnvironmentObject var appState: AppState
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            Group {
                if appState.pendingApprovals.isEmpty {
                    VStack(spacing: 12) {
                        Image(systemName: "checkmark.circle")
                            .font(.largeTitle)
                            .foregroundColor(.green)
                        Text("No pending approvals")
                            .foregroundColor(.secondary)
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else {
                    ScrollView {
                        VStack(spacing: 8) {
                            ForEach(appState.pendingApprovals) { event in
                                ApprovalRow(event: event)
                            }
                        }
                        .padding(12)
                    }
                }
            }
            .navigationTitle("Approvals")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
        }
    }
}
