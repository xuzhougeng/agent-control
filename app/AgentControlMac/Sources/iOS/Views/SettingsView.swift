import SwiftUI

struct SettingsView: View {
    @EnvironmentObject var appState: AppState
    @Environment(\.dismiss) private var dismiss

    @State private var baseURL = ""
    @State private var token = ""
    @State private var skipTLSVerify = false
    @State private var saved = false

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
                }

                Section {
                    Toggle("Skip TLS verification", isOn: $skipTLSVerify)
                } footer: {
                    Text("Enable only for self-signed certificates during development.")
                        .font(.caption)
                }

                Section {
                    Button {
                        appState.saveConfig(baseURL: baseURL, token: token, skipTLSVerify: skipTLSVerify)
                        saved = true
                        DispatchQueue.main.asyncAfter(deadline: .now() + 2) { saved = false }
                    } label: {
                        HStack {
                            Spacer()
                            if saved {
                                Label("Saved", systemImage: "checkmark.circle.fill")
                                    .foregroundColor(.green)
                            } else {
                                Text("Save & Reconnect")
                            }
                            Spacer()
                        }
                    }
                    .buttonStyle(.borderedProminent)
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
