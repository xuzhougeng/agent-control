import SwiftUI

struct HomeTabView: View {
    @EnvironmentObject var appState: AppState
    @Binding var selectedTab: AppTab
    @State private var showServerGuide = false
    @State private var searchText = ""
    @State private var confirmDeleteID: String?

    private var runningSessions: [Session] {
        appState.sessions.filter(\.isRunning).sessionFiltered(by: searchText)
    }

    private var stoppedSessions: [Session] {
        appState.sessions.filter { !$0.isRunning }.sessionFiltered(by: searchText)
    }

    var body: some View {
        ZStack {
            WorkspaceRootBackground()
                .ignoresSafeArea()
            ScrollView {
                VStack(spacing: 10) {
                    searchCard
                    connectionCard

                    if !appState.pendingApprovals.isEmpty {
                        approvalsCard
                    }

                    serversCard

                    if !runningSessions.isEmpty {
                        sessionsCard(title: "Running", sessions: runningSessions)
                    }

                    if !stoppedSessions.isEmpty {
                        sessionsCard(title: "Stopped", sessions: stoppedSessions)
                    }

                    if appState.sessions.isEmpty && !appState.servers.isEmpty {
                        emptySessionsCard
                    }
                }
                .padding(12)
            }
        }
        .refreshable {
            await appState.fetchServers()
            await appState.fetchSessions()
        }
        .navigationTitle("Workspace")
        .toolbar {
            ToolbarItem(placement: .navigationBarTrailing) {
                Button { appState.showNewSessionSheet = true } label: {
                    Image(systemName: "plus")
                }
                .disabled(appState.selectedServerID == nil)
                .accessibilityLabel("New Session")
            }
        }
        .sheet(isPresented: $showServerGuide) {
            ServerGuideSheet(onOpenSettings: { selectedTab = .settings })
        }
        .alert("Delete Session?", isPresented: Binding(
            get: { confirmDeleteID != nil },
            set: { if !$0 { confirmDeleteID = nil } }
        )) {
            Button("Cancel", role: .cancel) { confirmDeleteID = nil }
            Button("Delete", role: .destructive) {
                if let id = confirmDeleteID {
                    Task { await appState.deleteSession(id) }
                }
                confirmDeleteID = nil
            }
        } message: {
            Text("This session will be permanently removed.")
        }
    }

    private var searchCard: some View {
        WorkspacePanel(inset: 10) {
            HStack(spacing: 8) {
                Image(systemName: "magnifyingglass")
                    .foregroundColor(WorkspaceTheme.textSoft)
                TextField("Search by session ID or path", text: $searchText)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
            }
            .font(.system(size: 14))
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
            .background(
                RoundedRectangle(cornerRadius: 10, style: .continuous)
                    .fill(WorkspaceTheme.surfaceStrong.opacity(0.58))
                    .overlay(
                        RoundedRectangle(cornerRadius: 10, style: .continuous)
                            .stroke(WorkspaceTheme.border.opacity(0.72), lineWidth: 1)
                    )
            )
        }
    }

    private var connectionCard: some View {
        WorkspacePanel(inset: 10) {
            VStack(alignment: .leading, spacing: 9) {
                WorkspaceSectionTitle(text: "Connection")
                HStack(spacing: 10) {
                    Circle()
                        .fill(appState.wsConnected ? WorkspaceTheme.success : WorkspaceTheme.danger)
                        .frame(width: 9, height: 9)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(appState.wsConnected ? "Connected" : "Disconnected")
                            .font(.subheadline.weight(.semibold))
                            .foregroundColor(WorkspaceTheme.text)
                        Text(baseURLHost)
                            .font(.caption)
                            .foregroundColor(WorkspaceTheme.textMuted)
                    }
                    Spacer()
                    if !appState.wsConnected {
                        Button("Settings") { selectedTab = .settings }
                            .font(.caption)
                            .buttonStyle(.bordered)
                            .tint(WorkspaceTheme.accent)
                    }
                }

                if let hint = appState.connectionHint {
                    Text(hint)
                        .font(.caption)
                        .foregroundColor(WorkspaceTheme.warning)
                }
            }
        }
    }

    private var approvalsCard: some View {
        WorkspacePanel(inset: 10) {
            VStack(alignment: .leading, spacing: 8) {
                HStack {
                    WorkspaceSectionTitle(text: "Pending Approvals")
                    Spacer()
                    WorkspaceCountBadge(text: "\(appState.pendingApprovals.count)", color: WorkspaceTheme.warning)
                }
                ForEach(appState.pendingApprovals) { event in
                    ApprovalRow(event: event)
                }
            }
        }
    }

    private var serversCard: some View {
        WorkspacePanel(inset: 10) {
            VStack(alignment: .leading, spacing: 8) {
                HStack {
                    WorkspaceSectionTitle(text: "Servers")
                    Spacer()
                    Button { showServerGuide = true } label: {
                        Image(systemName: "questionmark.circle")
                    }
                    .buttonStyle(.plain)
                    Button {
                        Task { await appState.fetchServers() }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                    }
                    .buttonStyle(.plain)
                }

                if appState.servers.isEmpty {
                    VStack(spacing: 8) {
                        Text("No servers found")
                            .font(.subheadline)
                            .foregroundColor(WorkspaceTheme.textMuted)
                        Button("How to add a server") { showServerGuide = true }
                            .font(.caption)
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 8)
                } else {
                    ForEach(appState.servers) { server in
                        Button {
                            appState.selectedServerID = server.serverID
                            Task { await appState.fetchSessions() }
                        } label: {
                            ServerRow(server: server, isSelected: server.serverID == appState.selectedServerID)
                        }
                        .buttonStyle(.plain)
                    }
                }

                if !appState.servers.isEmpty {
                    Text("Servers come from the connected control plane.")
                        .font(.caption2)
                        .foregroundColor(WorkspaceTheme.textSoft)
                }
            }
        }
    }

    private func sessionsCard(title: String, sessions: [Session]) -> some View {
        WorkspacePanel(inset: 10) {
            VStack(alignment: .leading, spacing: 8) {
                HStack {
                    WorkspaceSectionTitle(text: title)
                    Spacer()
                    WorkspaceCountBadge(text: "\(sessions.count)", color: WorkspaceTheme.accent)
                }
                ForEach(sessions) { session in
                    HStack(alignment: .top, spacing: 8) {
                        Button {
                            appState.openSession(session)
                            selectedTab = session.isChat ? .chat : .terminal
                        } label: {
                            SessionRow(session: session, isSelected: isSelected(session))
                        }
                        .buttonStyle(.plain)

                        VStack(spacing: 6) {
                            if session.isPTY, session.isRunning {
                                Button {
                                    Task { await appState.stopSession(session.sessionID) }
                                } label: {
                                    Image(systemName: "stop.fill")
                                        .font(.system(size: 11, weight: .semibold))
                                }
                                .buttonStyle(.bordered)
                                .tint(WorkspaceTheme.warning)
                            }

                            Button(role: .destructive) {
                                confirmDeleteID = session.sessionID
                            } label: {
                                Image(systemName: "trash")
                                    .font(.system(size: 11, weight: .semibold))
                            }
                            .buttonStyle(.bordered)
                            .tint(WorkspaceTheme.danger)
                        }
                    }
                }
            }
        }
    }

    private var emptySessionsCard: some View {
        WorkspacePanel(inset: 10) {
            VStack(spacing: 12) {
                Image(systemName: "terminal")
                    .font(.largeTitle)
                    .foregroundColor(WorkspaceTheme.textSoft)
                Text("No sessions yet")
                    .font(.subheadline)
                    .foregroundColor(WorkspaceTheme.textMuted)
                Button {
                    appState.showNewSessionSheet = true
                } label: {
                    Label("New Session", systemImage: "plus")
                }
                .buttonStyle(.borderedProminent)
                .tint(WorkspaceTheme.accent)
                .controlSize(.small)
                .disabled(appState.selectedServerID == nil)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 16)
        }
    }

    private var baseURLHost: String {
        let url = UserDefaults.standard.string(forKey: "baseURL") ?? "http://127.0.0.1:18080"
        return URLComponents(string: url)?.host ?? url
    }

    private func isSelected(_ session: Session) -> Bool {
        if session.isChat {
            return session.sessionID == appState.selectedChatSessionID
        }
        return session.sessionID == appState.selectedSessionID
    }
}

// MARK: - Session search filter

extension Array where Element == Session {
    func sessionFiltered(by query: String) -> [Session] {
        guard !query.isEmpty else { return self }
        let q = query.lowercased()
        return filter {
            $0.shortID.lowercased().contains(q) ||
            $0.cwd.lowercased().contains(q)
        }
    }
}
