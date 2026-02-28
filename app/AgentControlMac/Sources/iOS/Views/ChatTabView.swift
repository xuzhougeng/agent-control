import SwiftUI

struct ChatTabView: View {
    @EnvironmentObject var appState: AppState
    @Binding var selectedTab: AppTab
    @State private var showSessionPanel = false

    private var isActive: Bool {
        selectedTab == .chat
    }

    var body: some View {
        ZStack {
            WorkspaceRootBackground()
                .ignoresSafeArea()
            ChatConversationView()
                .padding(12)
        }
            .navigationTitle("Chat Workspace")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                if isActive {
                    ToolbarItem(placement: .navigationBarLeading) {
                        Button {
                            showSessionPanel = true
                        } label: {
                            Image(systemName: "list.bullet")
                        }
                        .accessibilityLabel("Chat Sessions")
                    }
                    ToolbarItem(placement: .navigationBarTrailing) {
                        if appState.selectedChatSessionID == nil {
                            Text("No session")
                                .font(.caption)
                                .foregroundColor(.secondary)
                        } else {
                            Button {
                                Task { await appState.fetchSessions() }
                            } label: {
                                Image(systemName: "arrow.clockwise")
                            }
                            .accessibilityLabel("Refresh")
                        }
                    }
                }
            }
            .sheet(isPresented: $showSessionPanel) {
                NavigationStack {
                    ChatSessionPanelView()
                        .navigationTitle("Chat Rail")
                        .navigationBarTitleDisplayMode(.inline)
                        .toolbar {
                            ToolbarItem(placement: .confirmationAction) {
                                Button("Done") { showSessionPanel = false }
                            }
                        }
                }
                .environmentObject(appState)
            }
            .onAppear {
                appState.selectedPage = .chat
            }
    }
}
