import SwiftUI

struct ApprovalPanelView: View {
    @EnvironmentObject var appState: AppState

    var body: some View {
        let pending = appState.pendingApprovals
        if !pending.isEmpty {
            VStack(alignment: .leading, spacing: 6) {
                HStack {
                    Text("Pending Approvals")
                        .font(.system(size: 12, weight: .semibold))
                        .textCase(.uppercase)
                        .foregroundColor(.secondary)

                    Text("\(pending.count)")
                        .font(.system(size: 10, weight: .bold))
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1)
                        .background(Capsule().fill(Color.red))
                        .foregroundColor(.white)

                    Spacer()
                }

                ForEach(pending) { event in
                    ApprovalRow(event: event)
                }
            }
            .padding(10)
            .frame(maxHeight: 200)
        }
    }
}

struct ApprovalRow: View {
    @EnvironmentObject var appState: AppState
    let event: SessionEvent

    var body: some View {
        HStack(spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 4) {
                    Text(String(event.sessionID.prefix(8)))
                        .font(.system(size: 12, weight: .semibold, design: .monospaced))
                    Text("@")
                        .foregroundColor(.secondary)
                        .font(.caption)
                    Text(event.serverID)
                        .font(.caption)
                        .foregroundColor(.secondary)
                }
                if let excerpt = event.promptExcerpt, !excerpt.isEmpty {
                    Text(excerpt)
                        .font(.system(size: 11))
                        .foregroundColor(.secondary)
                        .lineLimit(2)
                }
            }

            Spacer()

            Button("Approve") {
                appState.attachSession(event.sessionID)
                appState.sendAction(sessionID: event.sessionID, kind: "approve")
            }
            .buttonStyle(.borderedProminent)
            .tint(.green)
            .controlSize(.small)

            Button("Reject") {
                appState.attachSession(event.sessionID)
                appState.sendAction(sessionID: event.sessionID, kind: "reject")
            }
            .buttonStyle(.bordered)
            .tint(.red)
            .controlSize(.small)
        }
        .padding(8)
        .background(
            RoundedRectangle(cornerRadius: 6)
                .fill(Color.yellow.opacity(0.08))
                .overlay(RoundedRectangle(cornerRadius: 6).strokeBorder(Color.yellow.opacity(0.2)))
        )
    }
}
