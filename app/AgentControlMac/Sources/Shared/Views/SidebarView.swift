import SwiftUI

struct SidebarView: View {
    @EnvironmentObject var appState: AppState
    @State private var showServerGuide = false
    let onOpenSettings: (() -> Void)?
    let onOpenSessionTerminal: (() -> Void)?

    init(
        onOpenSettings: (() -> Void)? = nil,
        onOpenSessionTerminal: (() -> Void)? = nil
    ) {
        self.onOpenSettings = onOpenSettings
        self.onOpenSessionTerminal = onOpenSessionTerminal
    }

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
                    #if os(macOS)
                    if #available(macOS 14, *) {
                        SettingsLink {
                            Image(systemName: "gearshape")
                        }
                        .buttonStyle(.plain)
                        .font(.caption)
                        .help("Connection Settings")
                    }
                    #else
                    if let onOpenSettings {
                        Button(action: onOpenSettings) {
                            Image(systemName: "gearshape")
                        }
                        .buttonStyle(.plain)
                        .font(.caption)
                        .help("Connection Settings")
                    }
                    #endif
                    Button { showServerGuide = true } label: {
                        Image(systemName: "questionmark.circle")
                    }
                    .buttonStyle(.plain)
                    .font(.caption)
                    .help("How to add server")
                    Button { Task { await appState.fetchServers() } } label: {
                        Image(systemName: "arrow.clockwise")
                    }
                    .buttonStyle(.plain)
                    .font(.caption)
                }
            } footer: {
                Text("Servers are discovered from the connected control plane. Tap ? for steps to add one.")
                    .font(.caption2)
                    .foregroundColor(.secondary)
            }

            // -- Sessions --
            Section {
                ForEach(appState.sessions) { session in
                    SessionRow(session: session, isSelected: session.sessionID == appState.selectedSessionID)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            appState.attachSession(session.sessionID)
                            onOpenSessionTerminal?()
                        }
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
        #if os(iOS)
        .refreshable {
            await appState.fetchServers()
            await appState.fetchSessions()
        }
        #endif
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
        .sheet(isPresented: $showServerGuide) {
            ServerGuideSheet(onOpenSettings: onOpenSettings)
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
        Button("Delete", role: .destructive) {
            Task { await appState.deleteSession(session.sessionID) }
        }
    }
}

struct ServerGuideSheet: View {
    @Environment(\.dismiss) private var dismiss
    let onOpenSettings: (() -> Void)?

    var body: some View {
        NavigationStack {
            List {
                Section("How server list works") {
                    Text("You do not create servers inside this app. The list comes from the control plane endpoint (/api/servers).")
                        .font(.subheadline)
                }
                Section("Add a server") {
                    Label("Open Settings and confirm Base URL + UI Token.", systemImage: "gearshape")
                    Label("Start cc-agent on target machine with a unique -server-id.", systemImage: "terminal")
                    Label("Return here and tap refresh in the Servers section.", systemImage: "arrow.clockwise")
                }
                #if os(macOS)
                if #available(macOS 14, *) {
                    Section {
                        SettingsLink {
                            Text("Open Settings")
                        }
                    }
                }
                #else
                if let onOpenSettings {
                    Section {
                        Button("Open Settings") {
                            dismiss()
                            onOpenSettings()
                        }
                    }
                }
                #endif
            }
            .navigationTitle("Add Server")
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
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
