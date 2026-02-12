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
            if appState.selectedSessionID != nil {
                VStack(spacing: 0) {
                    TerminalContainerView()
                }
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
                        isOpen: $showSessionDrawer
                    )
                    .environmentObject(appState)
                    .frame(width: min(UIScreen.main.bounds.width * 0.82, 340))
                    .transition(.move(edge: .leading))
                }
                Spacer(minLength: 0)
            }
        }
        .navigationTitle(currentSession.map { "Session \($0.shortID)" } ?? "Terminal")
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
        VStack(spacing: 16) {
            Image(systemName: "terminal")
                .font(.system(size: 48))
                .foregroundColor(.secondary)
            Text("No session selected")
                .font(.title3)
                .foregroundColor(.secondary)

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
                }
                Button {
                    appState.showNewSessionSheet = true
                } label: {
                    Label("New Session", systemImage: "plus")
                }
                .buttonStyle(.borderedProminent)
                .disabled(appState.selectedServerID == nil)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color(uiColor: .systemBackground))
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

            List {
                Section {
                    Button {
                        appState.showNewSessionSheet = true
                        withAnimation(.easeInOut(duration: 0.22)) {
                            isOpen = false
                        }
                    } label: {
                        Label("New Session", systemImage: "plus")
                    }
                    .foregroundColor(.primary)
                    .disabled(appState.selectedServerID == nil)
                }

                let running = appState.sessions.filter(\.isRunning)
                let stopped = appState.sessions.filter { !$0.isRunning }

                if !running.isEmpty {
                    Section("Running") {
                        ForEach(running) { session in
                            sessionButton(session)
                        }
                    }
                }
                if !stopped.isEmpty {
                    Section("Stopped") {
                        ForEach(stopped) { session in
                            sessionButton(session)
                        }
                    }
                }
                if appState.sessions.isEmpty {
                    Section {
                        Text("No sessions available")
                            .foregroundColor(.secondary)
                            .frame(maxWidth: .infinity, alignment: .center)
                    }
                }
            }
            .listStyle(.insetGrouped)
        }
        .background(Color(uiColor: .systemBackground))
    }

    private func sessionButton(_ session: Session) -> some View {
        Button {
            appState.attachSession(session.sessionID)
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
                if session.sessionID == appState.selectedSessionID {
                    Image(systemName: "checkmark")
                        .foregroundColor(.accentColor)
                }
                StatusBadge(label: session.status, isOnline: session.isRunning)
            }
        }
        .foregroundColor(.primary)
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
                    List {
                        ForEach(appState.pendingApprovals) { event in
                            ApprovalRow(event: event)
                        }
                    }
                    .listStyle(.insetGrouped)
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
