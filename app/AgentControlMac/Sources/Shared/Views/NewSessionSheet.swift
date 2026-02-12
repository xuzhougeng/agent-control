import SwiftUI

struct NewSessionSheet: View {
    @EnvironmentObject var appState: AppState
    @Environment(\.dismiss) private var dismiss
    var onCreated: (() -> Void)?

    @State private var cwd = ""
    @State private var resumeID = ""
    @State private var envString = ""
    @State private var isCreating = false

    private var selectedServer: Server? {
        guard let sid = appState.selectedServerID else { return nil }
        return appState.servers.first { $0.serverID == sid }
    }

    var body: some View {
        #if os(iOS)
        iOSBody
        #else
        macOSBody
        #endif
    }

    // MARK: - iOS (NavigationStack + Form)

    #if os(iOS)
    private var iOSBody: some View {
        NavigationStack {
            Form {
                Section {
                    HStack {
                        Text("Server")
                        Spacer()
                        Text(appState.selectedServerID ?? "none")
                            .font(.system(.body, design: .monospaced))
                            .foregroundColor(.secondary)
                    }
                }

                Section {
                    TextField("Working directory", text: $cwd)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                        .accessibilityLabel("Working directory path")

                    if let roots = selectedServer?.allowRoots, !roots.isEmpty {
                        ForEach(roots, id: \.self) { root in
                            Button {
                                cwd = root
                            } label: {
                                Label(root, systemImage: "folder")
                                    .font(.subheadline)
                            }
                            .foregroundColor(.primary)
                        }
                    }
                } header: {
                    Text("Working Directory")
                } footer: {
                    if cwd.isEmpty {
                        Text("Required. Must be an allowed root on the server.")
                    }
                }

                Section("Resume ID (optional)") {
                    TextField("UUID to resume a previous session", text: $resumeID)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                }

                Section {
                    TextField("CC_PROFILE=dev", text: $envString)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                } header: {
                    Text("Environment Variables")
                } footer: {
                    Text("Comma-separated KEY=VALUE pairs.")
                }
            }
            .navigationTitle("New Session")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .disabled(isCreating)
                }
                ToolbarItem(placement: .confirmationAction) {
                    if isCreating {
                        ProgressView()
                    } else {
                        Button("Create") { create() }
                            .fontWeight(.semibold)
                            .disabled(cwd.isEmpty || appState.selectedServerID == nil)
                    }
                }
            }
            .interactiveDismissDisabled(isCreating)
        }
    }
    #endif

    // MARK: - macOS (compact VStack — unchanged layout)

    #if os(macOS)
    private var macOSBody: some View {
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
                Button("Create") { create() }
                    .keyboardShortcut(.defaultAction)
                    .buttonStyle(.borderedProminent)
                    .disabled(cwd.isEmpty || appState.selectedServerID == nil || isCreating)
            }
        }
        .padding(20)
        .frame(width: 420)
    }
    #endif

    // MARK: - Shared logic

    private func create() {
        isCreating = true
        Task {
            await appState.createSession(
                cwd: cwd,
                resumeID: resumeID.isEmpty ? nil : resumeID,
                env: parseEnv(envString)
            )
            dismiss()
            onCreated?()
        }
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
