import SwiftUI

@main
struct AgentControlMacApp: App {
    @StateObject private var appState = AppState()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(appState)
                .preferredColorScheme(.dark)
                .frame(minWidth: 900, minHeight: 600)
                .onAppear { appState.start() }
        }
        .defaultSize(width: 1200, height: 800)

        Settings {
            SettingsView()
                .environmentObject(appState)
        }
    }
}
