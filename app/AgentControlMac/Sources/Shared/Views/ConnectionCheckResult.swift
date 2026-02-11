import SwiftUI

enum ConnectionCheckResult {
    case checking
    case ok
    case failed(String)

    @ViewBuilder
    var label: some View {
        switch self {
        case .checking:
            Label("Checking…", systemImage: "arrow.triangle.2.circlepath")
                .foregroundColor(.secondary)
        case .ok:
            Label("Connected", systemImage: "checkmark.circle.fill")
                .foregroundColor(.green)
        case .failed(let msg):
            Label(msg, systemImage: "xmark.circle.fill")
                .foregroundColor(.red)
                .lineLimit(2)
        }
    }
}
