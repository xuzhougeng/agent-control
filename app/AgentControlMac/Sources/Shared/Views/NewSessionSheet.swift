import SwiftUI

struct NewSessionSheet: View {
    @EnvironmentObject var appState: AppState
    @Environment(\.dismiss) private var dismiss

    @State private var cwd = ""
    @State private var resumeID = ""
    @State private var envString = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("New Session")
                .font(.headline)

            HStack(spacing: 4) {
                Text("Server:")
                    .foregroundColor(.secondary)
                Text(appState.selectedServerID ?? "none")
                    .font(.system(.body, design: .monospaced))
            }

            VStack(alignment: .leading, spacing: 4) {
                Text("Working directory").font(.caption).foregroundColor(.secondary)
                TextField("/path/to/repo", text: $cwd)
                    .textFieldStyle(.roundedBorder)
            }

            VStack(alignment: .leading, spacing: 4) {
                Text("Resume ID (optional)").font(.caption).foregroundColor(.secondary)
                TextField("uuid", text: $resumeID)
                    .textFieldStyle(.roundedBorder)
            }

            VStack(alignment: .leading, spacing: 4) {
                Text("Env (KEY=VALUE, comma-separated)").font(.caption).foregroundColor(.secondary)
                TextField("CC_PROFILE=dev", text: $envString)
                    .textFieldStyle(.roundedBorder)
            }

            HStack {
                Spacer()
                Button("Cancel") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button("Create") {
                    Task {
                        await appState.createSession(
                            cwd: cwd,
                            resumeID: resumeID.isEmpty ? nil : resumeID,
                            env: parseEnv(envString)
                        )
                        dismiss()
                    }
                }
                .keyboardShortcut(.defaultAction)
                .buttonStyle(.borderedProminent)
                .disabled(cwd.isEmpty || appState.selectedServerID == nil)
            }
        }
        .padding(20)
        #if os(macOS)
        .frame(width: 420)
        #endif
    }

    private func parseEnv(_ input: String) -> [String: String] {
        var env: [String: String] = [:]
        for pair in input.split(separator: ",") {
            let trimmed = pair.trimmingCharacters(in: .whitespaces)
            guard let idx = trimmed.firstIndex(of: "="), idx > trimmed.startIndex else { continue }
            env[String(trimmed[..<idx])] = String(trimmed[trimmed.index(after: idx)...])
        }
        return env
    }
}
