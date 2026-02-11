import SwiftUI

struct SidebarView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        List {
            // -- Servers --
            Section {
                ForEach(appState.servers) { server in
                    ServerRow(server: server, isSelected: server.serverID == appState.selectedServerID)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            appState.selectedServerID = server.serverID
                            Task { await appState.fetchSessions() }
                        }
                }
            } header: {
                HStack {
                    Text("Servers")
                    Spacer()
                    Button { Task { await appState.fetchServers() } } label: {
                        Image(systemName: "arrow.clockwise")
                    }
                    .buttonStyle(.plain)
                    .font(.caption)
                }
            }

            // -- Sessions --
            Section {
                ForEach(appState.sessions) { session in
                    SessionRow(session: session, isSelected: session.sessionID == appState.selectedSessionID)
                        .contentShape(Rectangle())
                        .onTapGesture { appState.attachSession(session.sessionID) }
                        .contextMenu { sessionContextMenu(session) }
                }
            } header: {
                HStack {
                    Text("Sessions")
                    Spacer()
                    Button { Task { await appState.fetchSessions() } } label: {
                        Image(systemName: "arrow.clockwise")
                    }
                    .buttonStyle(.plain)
                    .font(.caption)
                }
            }
        }
        .listStyle(.sidebar)
        .navigationTitle("Agent Control")
        .toolbar {
            ToolbarItem {
                Button { appState.showNewSessionSheet = true } label: {
                    Image(systemName: "plus")
                }
                .disabled(appState.selectedServerID == nil)
                .help("New Session")
            }
        }
    }

    @ViewBuilder
    private func sessionContextMenu(_ session: Session) -> some View {
        if let rid = session.resumeID, !rid.isEmpty {
            Button("Resume") { Task { await appState.resumeSession(session) } }
        }
        if session.isRunning {
            Button("Stop") { Task { await appState.stopSession(session.sessionID) } }
        }
    }
}

// MARK: - Row views

struct ServerRow: View {
    let server: Server
    let isSelected: Bool

    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                Text(server.serverID)
                    .font(.system(size: 13, weight: .semibold, design: .monospaced))
                if !server.hostname.isEmpty {
                    Text(server.hostname)
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
                if let tags = server.tags, !tags.isEmpty {
                    Text(tags.joined(separator: ", "))
                        .font(.caption2)
                        .foregroundColor(.secondary)
                }
            }
            Spacer()
            StatusBadge(label: server.status, isOnline: server.isOnline)
        }
        .padding(.vertical, 2)
        .listRowBackground(isSelected ? Color.accentColor.opacity(0.15) : Color.clear)
    }
}

struct SessionRow: View {
    let session: Session
    let isSelected: Bool

    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                Text(session.shortID)
                    .font(.system(size: 13, weight: .semibold, design: .monospaced))
                Text(session.cwd)
                    .font(.caption)
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                if let rid = session.resumeID, !rid.isEmpty {
                    Text("resume: \(String(rid.prefix(8)))")
                        .font(.caption2)
                        .foregroundColor(.secondary)
                }
                if session.awaitingApproval {
                    Text("approval: yes")
                        .font(.caption2)
                        .foregroundColor(.yellow)
                }
                if let reason = session.exitReason, !reason.isEmpty {
                    Text("reason: \(reason)")
                        .font(.caption2)
                        .foregroundColor(.secondary)
                }
            }
            Spacer()
            StatusBadge(label: session.status, isOnline: session.isRunning)
        }
        .padding(.vertical, 2)
        .listRowBackground(isSelected ? Color.accentColor.opacity(0.15) : Color.clear)
    }
}

struct StatusBadge: View {
    let label: String
    let isOnline: Bool

    var body: some View {
        Text(label)
            .font(.system(size: 10, weight: .semibold))
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(
                Capsule()
                    .fill(isOnline ? Color.green.opacity(0.12) : Color.red.opacity(0.12))
                    .overlay(Capsule().strokeBorder(isOnline ? Color.green.opacity(0.4) : Color.red.opacity(0.4)))
            )
            .foregroundColor(isOnline ? .green : .red)
    }
}
