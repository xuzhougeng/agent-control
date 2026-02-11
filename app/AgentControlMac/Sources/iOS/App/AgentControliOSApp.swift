import SwiftUI

@main
struct AgentControliOSApp: App {
    @StateObject private var appState = AppState()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(appState)
                .preferredColorScheme(.dark)
                .onAppear { appState.start() }
        }
        .onChange(of: scenePhase) { phase in
            switch phase {
            case .background:
                appState.pause()
            case .active:
                appState.resume()
            default:
                break
            }
        }
    }
}
