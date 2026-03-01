import SwiftUI

struct ChatTabView: View {
    @EnvironmentObject var appState: AppState
    @Binding var selectedTab: AppTab
    @State private var showSessionDrawer = false

    private var isActive: Bool {
        selectedTab == .chat
    }

    private var hasSessionInChatContext: Bool {
        appState.selectedChatSessionID != nil ||
        (appState.selectedPage == .chat && appState.selectedSessionID != nil)
    }

    var body: some View {
        ZStack {
            WorkspaceRootBackground()
                .ignoresSafeArea()
            ChatConversationView()
                .padding(12)

            if showSessionDrawer {
                Color.black.opacity(0.35)
                    .ignoresSafeArea()
                    .onTapGesture {
                        withAnimation(.easeInOut(duration: 0.22)) {
                            showSessionDrawer = false
                        }
                    }
                    .transition(.opacity)
            }

            HStack(spacing: 0) {
                if showSessionDrawer {
                    SessionDrawerView(
                        isOpen: $showSessionDrawer,
                        selectedTab: $selectedTab,
                        preferredTabAfterSelection: .chat
                    )
                    .environmentObject(appState)
                    .frame(width: min(UIScreen.main.bounds.width * 0.82, 340))
                    .transition(.move(edge: .leading))
                }
                Spacer(minLength: 0)
            }
        }
            .navigationTitle("Chat Workspace")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                if isActive {
                    ToolbarItem(placement: .navigationBarLeading) {
                        Button {
                            withAnimation(.easeInOut(duration: 0.22)) {
                                showSessionDrawer.toggle()
                            }
                        } label: {
                            Image(systemName: "list.bullet")
                        }
                        .accessibilityLabel("Chat Sessions")
                    }
                    ToolbarItem(placement: .navigationBarTrailing) {
                        if !hasSessionInChatContext {
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
            .animation(.easeInOut(duration: 0.22), value: showSessionDrawer)
            .onChange(of: selectedTab) { newValue in
                guard newValue != .chat else { return }
                showSessionDrawer = false
            }
            .onAppear {
                appState.selectedPage = .chat
            }
    }
}
