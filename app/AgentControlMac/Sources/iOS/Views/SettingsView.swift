import SwiftUI

struct SettingsView: View {
    @EnvironmentObject var appState: AppState
    @Environment(\.dismiss) private var dismiss

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
        NavigationStack {
            Form {
                Section("Connection") {
                    TextField("Base URL", text: $baseURL)
                        .keyboardType(.URL)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    SecureField("UI Token", text: $token)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                    if !baseURL.isEmpty && !isBaseURLValid {
                        Text("Invalid Base URL. Use format like http://127.0.0.1:18080")
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
                        .disabled(!canSubmit)
                        Spacer()
                        Button {
                            appState.saveConfig(baseURL: baseURL, token: token, skipTLSVerify: skipTLSVerify)
                            saved = true
                            DispatchQueue.main.asyncAfter(deadline: .now() + 2) { saved = false }
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
            }
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Done") { dismiss() }
                }
            }
        }
        .onAppear {
            baseURL = UserDefaults.standard.string(forKey: "baseURL") ?? "http://127.0.0.1:18080"
            token = KeychainHelper.load(key: "ui_token") ?? "admin-dev-token"
            skipTLSVerify = UserDefaults.standard.bool(forKey: "skipTLSVerify")
        }
    }
}
