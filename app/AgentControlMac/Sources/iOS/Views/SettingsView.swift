import SwiftUI

struct SettingsView: View {
    @EnvironmentObject var appState: AppState
    @Environment(\.dismiss) private var dismiss
    var showDismiss: Bool = true

    @State private var baseURL = ""
    @State private var token = ""
    @State private var skipTLSVerify = false
    @State private var saved = false
    @State private var connectionCheck: ConnectionCheckResult?

    private var isBaseURLValid: Bool {
        AppState.isValidBaseURL(baseURL)
    }

    private var canSubmit: Bool {
        isBaseURLValid && !token.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    private var isLoopbackBaseURL: Bool {
        guard let host = URLComponents(string: baseURL.trimmingCharacters(in: .whitespacesAndNewlines))?.host?.lowercased() else {
            return false
        }
        return host == "127.0.0.1" || host == "localhost" || host == "::1"
    }

    private var showDeviceLoopbackWarning: Bool {
        #if targetEnvironment(simulator)
        return false
        #else
        return isLoopbackBaseURL
        #endif
    }

    var body: some View {
        Group {
            if showDismiss {
                NavigationStack {
                    settingsForm
                        .navigationTitle("Settings")
                        .navigationBarTitleDisplayMode(.inline)
                        .toolbar {
                            ToolbarItem(placement: .cancellationAction) {
                                Button("Done") { dismiss() }
                            }
                            ToolbarItemGroup(placement: .keyboard) {
                                Spacer()
                                Button("Done") { hideKeyboard() }
                            }
                        }
                }
            } else {
                settingsForm
                    .navigationTitle("Settings")
                    .navigationBarTitleDisplayMode(.inline)
                    .toolbar {
                        ToolbarItemGroup(placement: .keyboard) {
                            Spacer()
                            Button("Done") { hideKeyboard() }
                        }
                    }
            }
        }
        .onAppear {
            baseURL = UserDefaults.standard.string(forKey: "baseURL") ?? "http://127.0.0.1:18080"
            token = KeychainHelper.load(key: "ui_token") ?? "admin-dev-token"
            skipTLSVerify = UserDefaults.standard.bool(forKey: "skipTLSVerify")
        }
    }

    private var settingsForm: some View {
        Form {
            Section("Connection") {
                TextField("Base URL", text: $baseURL)
                    .keyboardType(.URL)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityLabel("Server base URL")
                SecureField("UI Token", text: $token)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityLabel("Authentication token")
                if !baseURL.isEmpty && !isBaseURLValid {
                    Text("Invalid Base URL. Use format like http://192.168.x.x:18080")
                        .font(.caption)
                        .foregroundColor(.red)
                }
                if showDeviceLoopbackWarning {
                    Text("This iOS device cannot use localhost/127.0.0.1. Use your Mac/server LAN address instead.")
                        .font(.caption)
                        .foregroundColor(.orange)
                }
            }

            Section {
                Toggle("Skip TLS verification", isOn: $skipTLSVerify)
            } footer: {
                Text("Enable only for self-signed certificates during development.")
                    .font(.caption)
            }

            Section {
                if let result = connectionCheck {
                    HStack { result.label.font(.caption); Spacer() }
                }
                HStack {
                    Button("Check connection") {
                        checkConnection()
                    }
                    .disabled(!canSubmit)
                    Spacer()
                    Button {
                        save()
                    } label: {
                        HStack {
                            if saved {
                                Label("Saved", systemImage: "checkmark.circle.fill")
                                    .foregroundColor(.green)
                            } else {
                                Text("Save & Reconnect")
                            }
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(!canSubmit)
                }
            }

            Section("Status") {
                HStack {
                    Text("WebSocket")
                    Spacer()
                    ConnectionBadge(connected: appState.wsConnected)
                }
                HStack {
                    Text("Servers")
                    Spacer()
                    Text("\(appState.servers.count)")
                        .foregroundColor(.secondary)
                }
                HStack {
                    Text("Sessions")
                    Spacer()
                    Text("\(appState.sessions.count)")
                        .foregroundColor(.secondary)
                }
            }
        }
        .scrollDismissesKeyboard(.interactively)
    }

    private func checkConnection() {
        connectionCheck = .checking
        Task {
            do {
                try await APIClient.checkConnection(baseURL: baseURL, token: token, skipTLSVerify: skipTLSVerify)
                await MainActor.run { connectionCheck = .ok }
            } catch {
                await MainActor.run { connectionCheck = .failed(error.localizedDescription) }
            }
        }
    }

    private func save() {
        appState.saveConfig(baseURL: baseURL, token: token, skipTLSVerify: skipTLSVerify)
        saved = true
        // Auto-check after save
        checkConnection()
        DispatchQueue.main.asyncAfter(deadline: .now() + 2) { saved = false }
    }

    private func hideKeyboard() {
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
    }
}
