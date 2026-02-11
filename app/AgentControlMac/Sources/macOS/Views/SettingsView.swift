import SwiftUI

struct SettingsView: View {
    @EnvironmentObject var appState: AppState

    @State private var baseURL = ""
    @State private var token = ""
    @State private var skipTLSVerify = false
    @State private var saved = false
    @State private var connectionCheck: ConnectionCheckResult?

    var body: some View {
        Form {
            Section("Connection") {
                TextField("Base URL", text: $baseURL)
                    .textFieldStyle(.roundedBorder)
                SecureField("UI Token", text: $token)
                    .textFieldStyle(.roundedBorder)
            }

            Section {
                Toggle("Skip TLS verification (自签名证书)", isOn: $skipTLSVerify)
            }

            Section {
                if let result = connectionCheck {
                    HStack {
                        result.label.font(.caption)
                        Spacer()
                    }
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
                    .disabled(baseURL.isEmpty || token.isEmpty)
                    Spacer()
                    if saved {
                        Label("Saved", systemImage: "checkmark.circle.fill")
                            .foregroundColor(.green)
                            .font(.caption)
                    }
                    Button("Save & Reconnect") {
                        appState.saveConfig(baseURL: baseURL, token: token, skipTLSVerify: skipTLSVerify)
                        saved = true
                        DispatchQueue.main.asyncAfter(deadline: .now() + 2) { saved = false }
                    }
                    .buttonStyle(.borderedProminent)
                }
            }
        }
        .formStyle(.grouped)
        .padding()
        .frame(width: 450, height: 300)
        .onAppear {
            baseURL = UserDefaults.standard.string(forKey: "baseURL") ?? "http://127.0.0.1:18080"
            token = KeychainHelper.load(key: "ui_token") ?? "admin-dev-token"
            skipTLSVerify = UserDefaults.standard.bool(forKey: "skipTLSVerify")
        }
    }
}
