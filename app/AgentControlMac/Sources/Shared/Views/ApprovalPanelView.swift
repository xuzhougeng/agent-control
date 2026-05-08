import SwiftUI

struct ApprovalPanelView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        WorkspacePanel(inset: 10) {
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 8) {
                    WorkspaceSectionTitle(text: "Approvals")
                    Spacer()
                    WorkspaceCountBadge(text: "\(appState.pendingApprovals.count)", color: WorkspaceTheme.warning)
                }

                if appState.pendingApprovals.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Label("No pending approvals", systemImage: "checkmark.circle")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(WorkspaceTheme.success)
                        Text("Approval prompts from Claude Code sessions will appear here.")
                            .font(.system(size: 11))
                            .foregroundColor(WorkspaceTheme.textSoft)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.vertical, 8)
                } else {
                    ScrollView {
                        VStack(spacing: 8) {
                            ForEach(appState.pendingApprovals) { event in
                                ApprovalRow(event: event)
                            }
                        }
                        .padding(.vertical, 2)
                    }
                }
            }
        }
    }
}

struct ApprovalRow: View {
    @EnvironmentObject var appState: AppState
    let event: SessionEvent

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                Text(String(event.sessionID.prefix(8)))
                    .font(.system(size: 11, weight: .semibold, design: .monospaced))
                    .foregroundColor(WorkspaceTheme.text)
                Text("@")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(WorkspaceTheme.textSoft)
                Text(event.serverID)
                    .font(.system(size: 11))
                    .foregroundColor(WorkspaceTheme.textMuted)
                Spacer()
                if event.isFromCCAgent {
                    SessionTag(text: "cc-agent", tint: WorkspaceTheme.accent)
                }
                SessionTag(text: "pending", tint: WorkspaceTheme.warning)
            }

            if let excerpt = event.promptExcerpt, !excerpt.isEmpty {
                Text(excerpt)
                    .font(.system(size: 11))
                    .foregroundColor(WorkspaceTheme.textMuted)
                    .lineLimit(3)
            }

            HStack(spacing: 8) {
                Button("Approve") {
                    appState.attachSession(event.sessionID)
                    appState.sendAction(sessionID: event.sessionID, kind: "approve")
                }
                .buttonStyle(.borderedProminent)
                .tint(WorkspaceTheme.success)
                .controlSize(.small)

                Button("Reject") {
                    appState.attachSession(event.sessionID)
                    appState.sendAction(sessionID: event.sessionID, kind: "reject")
                }
                .buttonStyle(.bordered)
                .tint(WorkspaceTheme.danger)
                .controlSize(.small)

                Spacer()
            }
        }
        .padding(9)
        .background(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .fill(WorkspaceTheme.warningSoft.opacity(0.7))
                .overlay(
                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                        .stroke(WorkspaceTheme.warning.opacity(0.3), lineWidth: 1)
                )
        )
    }
}
