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
        List {
            connectionSection

            if !appState.pendingApprovals.isEmpty {
                approvalsSection
            }

            serversSection

            if !runningSessions.isEmpty {
                sessionsSection(title: "Running", sessions: runningSessions)
            }

            if !stoppedSessions.isEmpty {
                sessionsSection(title: "Stopped", sessions: stoppedSessions)
            }

            if appState.sessions.isEmpty && !appState.servers.isEmpty {
                emptySessionsSection
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle("Agent Control")
        .refreshable {
            await appState.fetchServers()
            await appState.fetchSessions()
        }
        .searchable(
            text: $searchText,
            placement: .navigationBarDrawer(displayMode: .always),
            prompt: "Search by session ID or path"
        )
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

    // MARK: - Connection card

    private var connectionSection: some View {
        Section {
            HStack(spacing: 12) {
                Circle()
                    .fill(appState.wsConnected ? Color.green : Color.red)
                    .frame(width: 10, height: 10)
                VStack(alignment: .leading, spacing: 2) {
                    Text(appState.wsConnected ? "Connected" : "Disconnected")
                        .font(.subheadline.weight(.semibold))
                    Text(baseURLHost)
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
                Spacer()
                if !appState.wsConnected {
                    Button("Settings") { selectedTab = .settings }
                        .font(.caption)
                        .buttonStyle(.bordered)
                }
            }
            .accessibilityElement(children: .combine)
            .accessibilityLabel(appState.wsConnected ? "Connected to \(baseURLHost)" : "Disconnected from \(baseURLHost)")

            if let hint = appState.connectionHint {
                Text(hint)
                    .font(.caption)
                    .foregroundColor(.orange)
            }
        }
    }

    // MARK: - Approvals

    private var approvalsSection: some View {
        Section {
            ForEach(appState.pendingApprovals) { event in
                ApprovalRow(event: event)
            }
        } header: {
            HStack {
                Text("Pending Approvals")
                Spacer()
                Text("\(appState.pendingApprovals.count)")
                    .font(.caption2.bold())
                    .padding(.horizontal, 6)
                    .padding(.vertical, 1)
                    .background(Capsule().fill(Color.red))
                    .foregroundColor(.white)
            }
        }
    }

    // MARK: - Servers

    private var serversSection: some View {
        Section {
            if appState.servers.isEmpty {
                VStack(spacing: 8) {
                    Text("No servers found")
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                    Button("How to add a server") { showServerGuide = true }
                        .font(.caption)
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 8)
            } else {
                ForEach(appState.servers) { server in
                    ServerRow(server: server, isSelected: server.serverID == appState.selectedServerID)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            appState.selectedServerID = server.serverID
                            Task { await appState.fetchSessions() }
                        }
                }
            }
        } header: {
            HStack {
                Text("Servers")
                Spacer()
                Button { showServerGuide = true } label: {
                    Image(systemName: "questionmark.circle")
                }
                .buttonStyle(.plain)
                .font(.caption)
            }
        } footer: {
            if !appState.servers.isEmpty {
                Text("Servers come from the connected control plane.")
                    .font(.caption2)
            }
        }
    }

    // MARK: - Sessions (grouped)

    private func sessionsSection(title: String, sessions: [Session]) -> some View {
        Section(title) {
            ForEach(sessions) { session in
                SessionRow(session: session, isSelected: session.sessionID == appState.selectedSessionID)
                    .contentShape(Rectangle())
                    .onTapGesture {
                        appState.attachSession(session.sessionID)
                        selectedTab = .terminal
                    }
                    .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                        Button(role: .destructive) {
                            confirmDeleteID = session.sessionID
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                        if session.isRunning {
                            Button {
                                Task { await appState.stopSession(session.sessionID) }
                            } label: {
                                Label("Stop", systemImage: "stop.fill")
                            }
                            .tint(.orange)
                        }
                    }
                    .swipeActions(edge: .leading, allowsFullSwipe: true) {
                        if let rid = session.resumeID, !rid.isEmpty, !session.isRunning {
                            Button {
                                Task {
                                    await appState.resumeSession(session)
                                    selectedTab = .terminal
                                }
                            } label: {
                                Label("Resume", systemImage: "play.fill")
                            }
                            .tint(.green)
                        }
                    }
            }
        }
    }

    // MARK: - Empty state

    private var emptySessionsSection: some View {
        Section {
            VStack(spacing: 12) {
                Image(systemName: "terminal")
                    .font(.largeTitle)
                    .foregroundColor(.secondary)
                Text("No sessions yet")
                    .font(.subheadline)
                    .foregroundColor(.secondary)
                Button {
                    appState.showNewSessionSheet = true
                } label: {
                    Label("New Session", systemImage: "plus")
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.small)
                .disabled(appState.selectedServerID == nil)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 16)
        }
    }

    // MARK: - Helpers

    private var baseURLHost: String {
        let url = UserDefaults.standard.string(forKey: "baseURL") ?? "http://127.0.0.1:18080"
        return URLComponents(string: url)?.host ?? url
    }
}

// MARK: - Session search filter

extension Array where Element == Session {
    func sessionFiltered(by query: String) -> [Session] {
        guard !query.isEmpty else { return self }
        let q = query.lowercased()
        return filter {
            $0.shortID.lowercased().contains(q) ||
            $0.cwd.lowercased().contains(q) ||
            ($0.resumeID?.lowercased().contains(q) ?? false)
        }
    }
}
