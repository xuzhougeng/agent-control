import SwiftUI

struct SidebarView: View {
    @EnvironmentObject var appState: AppState
    @State private var showServerGuide = false
    @State private var approvalsExpanded = false
    @State private var notificationsExpanded = false
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
        VStack(spacing: 8) {
            headerBar

            WorkspacePanel(inset: 6) {
                HStack(spacing: 8) {
                    pageButton(title: "Terminal", icon: "terminal", page: .terminal)
                    pageButton(title: "Chat", icon: "bubble.left.and.bubble.right", page: .chat)
                }
            }

            ScrollView {
                VStack(spacing: 8) {
                    serversPanel
                    sessionsPanel
                    approvalsPanel
                    notificationsPanel
                }
                .padding(.bottom, 6)
            }
            #if os(iOS)
            .refreshable {
                await appState.fetchServers()
                await appState.fetchSessions()
            }
            #endif
        }
        .padding(10)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(
            LinearGradient(
                colors: [WorkspaceTheme.bgBase.opacity(0.92), WorkspaceTheme.bgBaseStrong.opacity(0.92)],
                startPoint: .top,
                endPoint: .bottom
            )
        )
        .navigationTitle("Workspace")
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

    private var headerBar: some View {
        WorkspacePanel(inset: 8) {
            VStack(alignment: .leading, spacing: 2) {
                Text("Agent Control")
                    .font(.system(size: 17, weight: .bold))
                    .foregroundColor(WorkspaceTheme.text)
                Text("Native Workspace")
                    .font(.system(size: 9, weight: .semibold))
                    .tracking(1.2)
                    .foregroundColor(WorkspaceTheme.textSoft)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var serversPanel: some View {
        WorkspacePanel(inset: 8) {
            VStack(alignment: .leading, spacing: 7) {
                HStack(spacing: 8) {
                    WorkspaceSectionTitle(text: "Servers")
                    Spacer()
                    WorkspaceCountBadge(text: "\(appState.servers.count)", color: WorkspaceTheme.accent)
                    settingsShortcut
                    iconButton("questionmark.circle") {
                        showServerGuide = true
                    }
                    iconButton("arrow.clockwise") {
                        Task { await appState.fetchServers() }
                    }
                }

                if appState.servers.isEmpty {
                    Text("No servers found from control plane.")
                        .font(.caption)
                        .foregroundColor(WorkspaceTheme.textMuted)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.vertical, 6)
                } else {
                    VStack(spacing: 6) {
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
                }

                Text("Servers are registered by cc-agent and discovered from /api/servers.")
                    .font(.system(size: 11))
                    .foregroundColor(WorkspaceTheme.textSoft)
            }
        }
    }

    private var sessionsPanel: some View {
        let running = appState.sessions.filter(\.isRunning)
        let stopped = appState.sessions.filter { !$0.isRunning }

        return WorkspacePanel(inset: 8) {
            VStack(alignment: .leading, spacing: 7) {
                HStack(spacing: 8) {
                    WorkspaceSectionTitle(text: "Sessions")
                    Spacer()
                    WorkspaceCountBadge(text: "\(appState.sessions.count)", color: WorkspaceTheme.accent)
                    iconButton("arrow.clockwise") {
                        Task { await appState.fetchSessions() }
                    }
                }

                if appState.sessions.isEmpty {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("No sessions")
                            .font(.subheadline.weight(.semibold))
                            .foregroundColor(WorkspaceTheme.textMuted)
                        Text("Create a session from the + button after selecting a server.")
                            .font(.caption)
                            .foregroundColor(WorkspaceTheme.textSoft)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.vertical, 8)
                } else {
                    if !running.isEmpty {
                        sessionGroup(title: "Running", sessions: running)
                    }
                    if !stopped.isEmpty {
                        sessionGroup(title: "Stopped", sessions: stopped)
                    }
                }
            }
        }
    }

    private var approvalsPanel: some View {
        let pending = appState.pendingApprovals
        return WorkspacePanel(inset: 8) {
            VStack(alignment: .leading, spacing: 7) {
                HStack(spacing: 8) {
                    WorkspaceSectionTitle(text: "Approvals")
                    Spacer()
                    WorkspaceCountBadge(text: "\(pending.count)", color: WorkspaceTheme.warning)
                    Button {
                        withAnimation(.easeInOut(duration: 0.18)) {
                            approvalsExpanded.toggle()
                        }
                    } label: {
                        Image(systemName: approvalsExpanded ? "chevron.up" : "chevron.down")
                            .font(.system(size: 10, weight: .bold))
                            .foregroundColor(WorkspaceTheme.textMuted)
                            .frame(width: 18, height: 18)
                            .background(
                                RoundedRectangle(cornerRadius: 6, style: .continuous)
                                    .fill(WorkspaceTheme.surfaceStrong)
                                    .overlay(
                                        RoundedRectangle(cornerRadius: 6, style: .continuous)
                                            .stroke(WorkspaceTheme.border.opacity(0.8), lineWidth: 1)
                                    )
                            )
                    }
                    .buttonStyle(.plain)
                    .disabled(pending.isEmpty)
                }

                if pending.isEmpty {
                    Text("No pending approvals")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(WorkspaceTheme.success)
                } else if approvalsExpanded {
                    VStack(spacing: 5) {
                        ForEach(pending) { event in
                            SidebarApprovalRow(event: event)
                        }
                    }
                } else {
                    Text("\(pending.count) pending approval(s). Expand to review and action.")
                        .font(.system(size: 10))
                        .foregroundColor(WorkspaceTheme.textSoft)
                }
            }
        }
    }

    private var notificationsPanel: some View {
        let notices = Array(appState.recentNotifications.prefix(20))
        return WorkspacePanel(inset: 8) {
            VStack(alignment: .leading, spacing: 7) {
                HStack(spacing: 8) {
                    WorkspaceSectionTitle(text: "Notifications")
                    Spacer()
                    WorkspaceCountBadge(text: "\(appState.recentNotifications.count)", color: WorkspaceTheme.accent)
                    Button {
                        withAnimation(.easeInOut(duration: 0.18)) {
                            notificationsExpanded.toggle()
                        }
                    } label: {
                        Image(systemName: notificationsExpanded ? "chevron.up" : "chevron.down")
                            .font(.system(size: 10, weight: .bold))
                            .foregroundColor(WorkspaceTheme.textMuted)
                            .frame(width: 18, height: 18)
                            .background(
                                RoundedRectangle(cornerRadius: 6, style: .continuous)
                                    .fill(WorkspaceTheme.surfaceStrong)
                                    .overlay(
                                        RoundedRectangle(cornerRadius: 6, style: .continuous)
                                            .stroke(WorkspaceTheme.border.opacity(0.8), lineWidth: 1)
                                    )
                            )
                    }
                    .buttonStyle(.plain)
                    .disabled(notices.isEmpty)
                }

                if notices.isEmpty {
                    Text("No notifications")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(WorkspaceTheme.textSoft)
                } else if notificationsExpanded {
                    VStack(spacing: 5) {
                        ForEach(notices) { notification in
                            Button {
                                openNotification(notification)
                            } label: {
                                NotificationRow(notification: notification)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                } else {
                    Text("\(notices.count) recent notification(s). Expand to inspect.")
                        .font(.system(size: 10))
                        .foregroundColor(WorkspaceTheme.textSoft)
                }
            }
        }
    }

    private func sessionGroup(title: String, sessions: [Session]) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            WorkspaceSectionTitle(text: title)
            ForEach(sessions) { session in
                Button {
                    handleSessionSelection(session)
                } label: {
                    SessionRow(session: session, isSelected: isSelected(session))
                }
                .buttonStyle(.plain)
                .contextMenu { sessionContextMenu(session) }
            }
        }
        .padding(.top, 2)
    }

    private var settingsShortcut: some View {
        Group {
            #if os(macOS)
            if #available(macOS 14, *) {
                SettingsLink {
                    Image(systemName: "gearshape")
                }
                .buttonStyle(.plain)
            }
            #else
            if let onOpenSettings {
                iconButton("gearshape") {
                    onOpenSettings()
                }
            }
            #endif
        }
    }

    private func pageButton(title: String, icon: String, page: AppDetailPage) -> some View {
        Button {
            appState.selectedPage = page
        } label: {
            HStack(spacing: 6) {
                Image(systemName: icon)
                Text(title)
                    .lineLimit(1)
            }
            .font(.system(size: 12, weight: .semibold))
            .foregroundColor(appState.selectedPage == page ? WorkspaceTheme.accent : WorkspaceTheme.textMuted)
            .frame(maxWidth: .infinity)
            .padding(.vertical, 6)
            .background(
                RoundedRectangle(cornerRadius: WorkspaceTheme.cornerRadiusSmall, style: .continuous)
                    .fill(appState.selectedPage == page ? WorkspaceTheme.accentSoft : WorkspaceTheme.surfaceStrong.opacity(0.55))
                    .overlay(
                        RoundedRectangle(cornerRadius: WorkspaceTheme.cornerRadiusSmall, style: .continuous)
                            .stroke(appState.selectedPage == page ? WorkspaceTheme.accent.opacity(0.45) : WorkspaceTheme.border.opacity(0.55), lineWidth: 1)
                    )
            )
        }
        .buttonStyle(.plain)
    }

    private func handleSessionSelection(_ session: Session) {
        if appState.selectedPage == .terminal {
            if session.isPTY {
                appState.attachSession(session.sessionID)
                onOpenSessionTerminal?()
                return
            }
            Task {
                do {
                    try await appState.switchSessionToPTY(session.sessionID)
                    await MainActor.run {
                        onOpenSessionTerminal?()
                    }
                } catch {
                    print("[ui] switchSessionToPTY failed: \(error)")
                }
            }
            return
        }

        if session.isChat {
            appState.openSession(session)
        } else {
            appState.selectSessionForChatView(session.sessionID)
        }
    }

    private func openNotification(_ notification: NotificationEvent) {
        guard let sid = notification.sessionID, !sid.isEmpty else { return }
        if let session = appState.sessions.first(where: { $0.sessionID == sid }) {
            appState.openSession(session)
            onOpenSessionTerminal?()
            return
        }
        appState.attachSession(sid)
        onOpenSessionTerminal?()
    }

    private func iconButton(_ icon: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: icon)
                .font(.system(size: 12, weight: .semibold))
                .foregroundColor(WorkspaceTheme.textMuted)
                .frame(width: 26, height: 26)
                .background(
                    RoundedRectangle(cornerRadius: 7, style: .continuous)
                        .fill(WorkspaceTheme.surfaceStrong.opacity(0.62))
                        .overlay(
                            RoundedRectangle(cornerRadius: 7, style: .continuous)
                                .stroke(WorkspaceTheme.border.opacity(0.7), lineWidth: 1)
                        )
                )
        }
        .buttonStyle(.plain)
    }

    @ViewBuilder
    private func sessionContextMenu(_ session: Session) -> some View {
        if session.isPTY, session.isRunning {
            Button("Stop") { Task { await appState.stopSession(session.sessionID) } }
        }
        Button("Delete", role: .destructive) {
            Task { await appState.deleteSession(session.sessionID) }
        }
    }

    private func isSelected(_ session: Session) -> Bool {
        if session.isChat {
            return session.sessionID == appState.selectedChatSessionID
        }
        return session.sessionID == appState.selectedSessionID
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
        HStack(spacing: 10) {
            Circle()
                .fill(server.isOnline ? WorkspaceTheme.success : WorkspaceTheme.textSoft.opacity(0.65))
                .frame(width: 7, height: 7)
            VStack(alignment: .leading, spacing: 3) {
                Text(server.serverID)
                    .font(.system(size: 12, weight: .semibold, design: .monospaced))
                    .foregroundColor(WorkspaceTheme.text)
                    .lineLimit(1)
                HStack(spacing: 6) {
                    if !server.hostname.isEmpty {
                        Text(server.hostname)
                    }
                    if let tags = server.tags, !tags.isEmpty {
                        Text(tags.joined(separator: ", "))
                    }
                }
                .font(.system(size: 11))
                .foregroundColor(WorkspaceTheme.textMuted)
                .lineLimit(1)
            }
            Spacer()
            StatusBadge(label: server.status, isOnline: server.isOnline)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 7)
        .background(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .fill(isSelected ? WorkspaceTheme.accentSoft : WorkspaceTheme.surfaceStrong.opacity(0.64))
                .overlay(
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .stroke(isSelected ? WorkspaceTheme.accent.opacity(0.5) : WorkspaceTheme.border.opacity(0.6), lineWidth: 1)
                )
        )
    }
}

struct SessionRow: View {
    let session: Session
    let isSelected: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(alignment: .center, spacing: 8) {
                Circle()
                    .fill(session.isRunning ? WorkspaceTheme.success : WorkspaceTheme.textSoft.opacity(0.6))
                    .frame(width: 6, height: 6)
                Text(session.shortID)
                    .font(.system(size: 12, weight: .semibold, design: .monospaced))
                    .foregroundColor(WorkspaceTheme.text)
                Spacer()
                StatusBadge(label: session.status, isOnline: session.isRunning)
            }

            Text(session.cwd)
                .font(.system(size: 11))
                .foregroundColor(WorkspaceTheme.textMuted)
                .lineLimit(1)

            HStack(spacing: 6) {
                SessionTag(text: session.isChat ? "chat" : "pty", tint: WorkspaceTheme.accent)
                if session.awaitingApproval {
                    SessionTag(text: "approval", tint: WorkspaceTheme.warning)
                }
                if let reason = session.exitReason, !reason.isEmpty {
                    Text(reason)
                        .font(.system(size: 10))
                        .foregroundColor(WorkspaceTheme.textSoft)
                        .lineLimit(1)
                }
            }
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 7)
        .background(
            RoundedRectangle(cornerRadius: 11, style: .continuous)
                .fill(isSelected ? WorkspaceTheme.accentSoft : WorkspaceTheme.surfaceStrong.opacity(0.56))
                .overlay(
                    RoundedRectangle(cornerRadius: 11, style: .continuous)
                        .stroke(isSelected ? WorkspaceTheme.accent.opacity(0.5) : WorkspaceTheme.border.opacity(0.52), lineWidth: 1)
                )
        )
    }
}

struct SessionTag: View {
    let text: String
    let tint: Color

    var body: some View {
        Text(text)
            .font(.system(size: 10, weight: .semibold))
            .foregroundColor(tint)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(
                Capsule(style: .continuous)
                    .fill(tint.opacity(0.12))
                    .overlay(Capsule(style: .continuous).stroke(tint.opacity(0.28), lineWidth: 1))
            )
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
                Capsule(style: .continuous)
                    .fill(isOnline ? WorkspaceTheme.successSoft : WorkspaceTheme.dangerSoft)
                    .overlay(Capsule(style: .continuous).stroke(isOnline ? WorkspaceTheme.success.opacity(0.3) : WorkspaceTheme.danger.opacity(0.3), lineWidth: 1))
            )
            .foregroundColor(isOnline ? WorkspaceTheme.success : WorkspaceTheme.danger)
    }
}

struct NotificationRow: View {
    let notification: NotificationEvent

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(spacing: 6) {
                Image(systemName: iconName)
                    .font(.system(size: 10, weight: .semibold))
                    .foregroundColor(levelColor)
                Text(notification.displayTitle)
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(WorkspaceTheme.text)
                    .lineLimit(1)
                Spacer()
                if let sid = notification.sessionID, !sid.isEmpty {
                    Text(String(sid.prefix(8)))
                        .font(.system(size: 10, weight: .semibold, design: .monospaced))
                        .foregroundColor(WorkspaceTheme.accent)
                }
            }
            Text(notification.message)
                .font(.system(size: 10))
                .foregroundColor(WorkspaceTheme.textMuted)
                .lineLimit(2)
            HStack(spacing: 6) {
                if let source = notification.source, !source.isEmpty {
                    Text(source)
                        .font(.system(size: 9, weight: .semibold))
                        .foregroundColor(WorkspaceTheme.textSoft)
                }
                if let serverID = notification.serverID, !serverID.isEmpty {
                    Text("@\(serverID)")
                        .font(.system(size: 9))
                        .foregroundColor(WorkspaceTheme.textSoft)
                }
            }
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
        .background(
            RoundedRectangle(cornerRadius: 9, style: .continuous)
                .fill(levelColor.opacity(0.12))
                .overlay(
                    RoundedRectangle(cornerRadius: 9, style: .continuous)
                        .stroke(levelColor.opacity(0.28), lineWidth: 1)
                )
        )
    }

    private var levelColor: Color {
        switch notification.level {
        case .success: return WorkspaceTheme.success
        case .warning: return WorkspaceTheme.warning
        case .error: return WorkspaceTheme.danger
        case .info: return WorkspaceTheme.accent
        }
    }

    private var iconName: String {
        switch notification.level {
        case .success: return "checkmark.circle.fill"
        case .warning: return "exclamationmark.triangle.fill"
        case .error: return "xmark.octagon.fill"
        case .info: return "bell.fill"
        }
    }
}

private struct SidebarApprovalRow: View {
    @EnvironmentObject var appState: AppState
    let event: SessionEvent

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 6) {
                Text(String(event.sessionID.prefix(8)))
                    .font(.system(size: 11, weight: .semibold, design: .monospaced))
                    .foregroundColor(WorkspaceTheme.text)
                Spacer()
                Button {
                    appState.attachSession(event.sessionID)
                    appState.sendAction(sessionID: event.sessionID, kind: "approve")
                } label: {
                    Image(systemName: "checkmark")
                        .font(.system(size: 10, weight: .bold))
                        .foregroundColor(.white)
                        .frame(width: 18, height: 18)
                        .background(Circle().fill(WorkspaceTheme.success))
                }
                .buttonStyle(.plain)

                Button {
                    appState.attachSession(event.sessionID)
                    appState.sendAction(sessionID: event.sessionID, kind: "reject")
                } label: {
                    Image(systemName: "xmark")
                        .font(.system(size: 10, weight: .bold))
                        .foregroundColor(.white)
                        .frame(width: 18, height: 18)
                        .background(Circle().fill(WorkspaceTheme.danger))
                }
                .buttonStyle(.plain)
            }
            if let excerpt = event.promptExcerpt, !excerpt.isEmpty {
                Text(excerpt)
                    .font(.system(size: 10))
                    .foregroundColor(WorkspaceTheme.textMuted)
                    .lineLimit(2)
            }
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
        .background(
            RoundedRectangle(cornerRadius: 9, style: .continuous)
                .fill(WorkspaceTheme.warningSoft.opacity(0.62))
                .overlay(
                    RoundedRectangle(cornerRadius: 9, style: .continuous)
                        .stroke(WorkspaceTheme.warning.opacity(0.32), lineWidth: 1)
                )
        )
    }
}
