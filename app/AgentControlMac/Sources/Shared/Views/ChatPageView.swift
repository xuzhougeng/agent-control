import SwiftUI
import UniformTypeIdentifiers

#if os(iOS)
import PhotosUI
import UIKit
#elseif os(macOS)
import AppKit
#endif

struct ChatDetailView: View {
    var body: some View {
        #if os(macOS)
        HStack(spacing: 12) {
            ChatSessionPanelView()
                .frame(minWidth: 280, idealWidth: 320, maxWidth: 360)
            ChatConversationView()
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        #else
        ChatConversationView()
        #endif
    }
}

#if os(macOS)
struct ChatSessionPanelView: View {
    @EnvironmentObject var appState: AppState
    @State private var cwd = ""
    @State private var envString = ""
    @State private var isCreating = false
    @State private var errorText: String?

    var body: some View {
        WorkspacePanel(inset: 10) {
            VStack(alignment: .leading, spacing: 10) {
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("Chat Workspace")
                            .font(.system(size: 18, weight: .bold))
                            .foregroundColor(WorkspaceTheme.text)
                        Text("Server + Session Rail")
                            .font(.system(size: 10, weight: .semibold))
                            .tracking(1.1)
                            .foregroundColor(WorkspaceTheme.textSoft)
                    }
                    Spacer()
                    Button {
                        Task {
                            await appState.fetchServers()
                            await appState.fetchSessions()
                        }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                            .font(.system(size: 12, weight: .semibold))
                            .foregroundColor(WorkspaceTheme.textMuted)
                            .frame(width: 30, height: 30)
                            .background(
                                RoundedRectangle(cornerRadius: 8, style: .continuous)
                                    .fill(WorkspaceTheme.surfaceStrong.opacity(0.68))
                                    .overlay(
                                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                                            .stroke(WorkspaceTheme.border.opacity(0.72), lineWidth: 1)
                                    )
                            )
                    }
                    .buttonStyle(.plain)
                    .help("Refresh")
                }

                ScrollView {
                    VStack(alignment: .leading, spacing: 10) {
                        serversSection
                        newChatSection
                        sessionsSection
                    }
                }

                if let errorText, !errorText.isEmpty {
                    Text(errorText)
                        .font(.caption)
                        .foregroundColor(WorkspaceTheme.danger)
                }
            }
        }
    }

    private var serversSection: some View {
        chatSectionCard(title: "Servers") {
            if appState.servers.isEmpty {
                Text("No servers")
                    .foregroundColor(WorkspaceTheme.textSoft)
                    .font(.caption)
            } else {
                ForEach(appState.servers) { server in
                    Button {
                        appState.selectedServerID = server.serverID
                        Task { await appState.fetchSessions() }
                    } label: {
                        HStack(spacing: 8) {
                            Circle()
                                .fill(server.isOnline ? WorkspaceTheme.success : WorkspaceTheme.textSoft.opacity(0.6))
                                .frame(width: 7, height: 7)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(server.serverID)
                                    .font(.system(.subheadline, design: .monospaced))
                                    .foregroundColor(WorkspaceTheme.text)
                                if !server.hostname.isEmpty {
                                    Text(server.hostname)
                                        .font(.caption2)
                                        .foregroundColor(WorkspaceTheme.textMuted)
                                }
                            }
                            Spacer()
                            StatusBadge(label: server.status, isOnline: server.isOnline)
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 7)
                        .background(
                            RoundedRectangle(cornerRadius: 9, style: .continuous)
                                .fill(server.serverID == appState.selectedServerID ? WorkspaceTheme.accentSoft : WorkspaceTheme.surfaceStrong.opacity(0.5))
                                .overlay(
                                    RoundedRectangle(cornerRadius: 9, style: .continuous)
                                        .stroke(server.serverID == appState.selectedServerID ? WorkspaceTheme.accent.opacity(0.48) : WorkspaceTheme.border.opacity(0.6), lineWidth: 1)
                                )
                        )
                    }
                    .buttonStyle(.plain)
                }
            }
        }
    }

    private var newChatSection: some View {
        chatSectionCard(title: "New Chat") {
            TextField("/path/to/repo", text: $cwd)
                .textFieldStyle(.roundedBorder)
            TextField("CC_PROFILE=dev", text: $envString)
                .textFieldStyle(.roundedBorder)
            HStack {
                Spacer()
                Button {
                    createChat()
                } label: {
                    if isCreating {
                        ProgressView()
                    } else {
                        Text("Create")
                    }
                }
                .buttonStyle(.borderedProminent)
                .tint(WorkspaceTheme.accent)
                .disabled(isCreating || cwd.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || appState.selectedServerID == nil)
            }
        }
    }

    private var sessionsSection: some View {
        chatSectionCard(title: "Chat Sessions") {
            if appState.chatSessions.isEmpty {
                Text("No chat sessions")
                    .foregroundColor(WorkspaceTheme.textSoft)
                    .font(.caption)
            } else {
                ForEach(appState.chatSessions) { session in
                    HStack(alignment: .top, spacing: 8) {
                        Button {
                            Task { await appState.attachChatSession(session.sessionID) }
                        } label: {
                            VStack(alignment: .leading, spacing: 4) {
                                HStack(spacing: 6) {
                                    Text(session.shortID)
                                        .font(.system(.subheadline, design: .monospaced))
                                        .foregroundColor(WorkspaceTheme.text)
                                    StatusBadge(label: session.status, isOnline: session.isRunning)
                                }
                                Text(session.cwd)
                                    .font(.caption)
                                    .foregroundColor(WorkspaceTheme.textMuted)
                                    .lineLimit(1)
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(.horizontal, 8)
                            .padding(.vertical, 7)
                            .background(
                                RoundedRectangle(cornerRadius: 9, style: .continuous)
                                    .fill(session.sessionID == appState.selectedChatSessionID ? WorkspaceTheme.accentSoft : WorkspaceTheme.surfaceStrong.opacity(0.52))
                                    .overlay(
                                        RoundedRectangle(cornerRadius: 9, style: .continuous)
                                            .stroke(session.sessionID == appState.selectedChatSessionID ? WorkspaceTheme.accent.opacity(0.48) : WorkspaceTheme.border.opacity(0.6), lineWidth: 1)
                                    )
                            )
                        }
                        .buttonStyle(.plain)
                        Button(role: .destructive) {
                            Task { await appState.deleteSession(session.sessionID) }
                        } label: {
                            Image(systemName: "trash")
                        }
                        .disabled(session.isRunning)
                        .help(session.isRunning ? "Cannot delete a running session" : "Delete")
                    }
                }
            }
        }
    }

    private func chatSectionCard<Content: View>(title: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            WorkspaceSectionTitle(text: title)
            content()
        }
        .padding(9)
        .background(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .fill(WorkspaceTheme.surfaceStrong.opacity(0.58))
                .overlay(
                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                        .stroke(WorkspaceTheme.border.opacity(0.65), lineWidth: 1)
                )
        )
    }

    private func createChat() {
        let trimmed = cwd.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            errorText = "Working directory is required"
            return
        }
        errorText = nil
        isCreating = true
        Task {
            await appState.createChatSession(cwd: trimmed, sessionID: nil, env: parseEnv(envString))
            await MainActor.run {
                isCreating = false
            }
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
#endif

struct ChatConversationView: View {
    @EnvironmentObject var appState: AppState
    @State private var draftText = ""
    @State private var attachments: [ChatAttachment] = []
    @State private var localErrorText: String?
    @State private var copiedSessionID = false
    @State private var permissionExpanded = false
    @State private var permissionMode = "dontAsk"
    @State private var permissionAllowedTools = ""
    @State private var permissionDisallowedTools = ""
    @State private var applyingPermissions = false
    @State private var switchingToChat = false
    @State private var showPermissionConfirm = false

    #if os(iOS)
    @State private var pickerItems: [PhotosPickerItem] = []
    @FocusState private var composerFocused: Bool
    #elseif os(macOS)
    @State private var showImageImporter = false
    #endif

    private var selectedSessionForConversation: Session? {
        if let chatID = appState.selectedChatSessionID,
           let chatSession = appState.sessions.first(where: { $0.sessionID == chatID }) {
            return chatSession
        }
        guard appState.selectedPage == .chat,
              let sessionID = appState.selectedSessionID else {
            return nil
        }
        return appState.sessions.first(where: { $0.sessionID == sessionID })
    }

    private var selectedSessionIDForConversation: String? {
        selectedSessionForConversation?.sessionID
    }

    private var selectedSessionIsChat: Bool {
        selectedSessionForConversation?.isChat == true
    }

    var body: some View {
        WorkspacePanel(inset: 0) {
            VStack(spacing: 0) {
                sessionInfoBar
                if selectedSessionIsChat {
                    permissionBar
                }
                if appState.chatRunState != .idle {
                    runStateBar
                }
                messageList
                if !attachments.isEmpty {
                    attachmentList
                }
                if let localErrorText, !localErrorText.isEmpty {
                    Text(localErrorText)
                        .font(.caption)
                        .foregroundColor(WorkspaceTheme.danger)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 6)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                inputBar
            }
        }
        .alert("Apply permission settings?", isPresented: $showPermissionConfirm) {
            Button("Cancel", role: .cancel) {}
            Button("Apply", role: .destructive) {
                applyPermissionSettings()
            }
        } message: {
            Text("This will restart the current chat session to apply new permissions.")
        }
        #if os(macOS)
        .fileImporter(
            isPresented: $showImageImporter,
            allowedContentTypes: [.image],
            allowsMultipleSelection: true
        ) { result in
            handleFileImport(result: result)
        }
        #endif
    }

    private var sessionInfoBar: some View {
        HStack(spacing: 8) {
            Text(selectedSessionIDForConversation.map { "Session: \(String($0.prefix(8)))" } ?? "Session: (none)")
                .font(.system(size: 11, design: .monospaced))
                .foregroundColor(WorkspaceTheme.textMuted)
                .lineLimit(1)
            Spacer()
            Button(copiedSessionID ? "Copied" : "Copy") {
                copySessionID()
            }
            .buttonStyle(.bordered)
            .controlSize(.small)
            .tint(WorkspaceTheme.accent)
            .disabled(selectedSessionIDForConversation == nil)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(WorkspaceTheme.surfaceStrong.opacity(0.95))
        .overlay(alignment: .bottom) {
            Rectangle()
                .fill(WorkspaceTheme.border.opacity(0.65))
                .frame(height: 1)
        }
    }

    private var permissionBar: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Permission Settings")
                    .font(.subheadline.weight(.semibold))
                    .foregroundColor(WorkspaceTheme.text)
                Spacer()
                Button(permissionExpanded ? "▲" : "▼") {
                    permissionExpanded.toggle()
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .tint(WorkspaceTheme.accent)
            }

            if permissionExpanded {
                VStack(alignment: .leading, spacing: 8) {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("permission_mode")
                            .font(.caption)
                            .foregroundColor(WorkspaceTheme.textSoft)
                        Picker("permission_mode", selection: $permissionMode) {
                            Text("dontAsk").tag("dontAsk")
                            Text("acceptEdits").tag("acceptEdits")
                            Text("bypassPermissions").tag("bypassPermissions")
                        }
                        .pickerStyle(.menu)
                    }
                    VStack(alignment: .leading, spacing: 4) {
                        Text("allowed_tools")
                            .font(.caption)
                            .foregroundColor(WorkspaceTheme.textSoft)
                        TextField("Bash(git:*) Read Edit", text: $permissionAllowedTools)
                            .textFieldStyle(.roundedBorder)
                    }
                    VStack(alignment: .leading, spacing: 4) {
                        Text("disallowed_tools")
                            .font(.caption)
                            .foregroundColor(WorkspaceTheme.textSoft)
                        TextField("", text: $permissionDisallowedTools)
                            .textFieldStyle(.roundedBorder)
                    }
                    HStack {
                        Spacer()
                        Button {
                            showPermissionConfirm = true
                        } label: {
                            if applyingPermissions {
                                ProgressView()
                            } else {
                                Text("Apply (Restart Session)")
                            }
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(WorkspaceTheme.accent)
                        .disabled(applyingPermissions || appState.selectedChatSessionID == nil)
                    }
                }
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .background(WorkspaceTheme.surfaceStrong.opacity(0.82))
        .overlay(alignment: .bottom) {
            Rectangle()
                .fill(WorkspaceTheme.border.opacity(0.65))
                .frame(height: 1)
        }
    }

    private var runStateBar: some View {
        Text(appState.chatRunState.text)
            .font(.caption)
            .foregroundColor(runStateColor)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 12)
            .padding(.vertical, 5)
            .background(runStateColor.opacity(0.10))
    }

    private var messageList: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 10) {
                    if appState.chatMessages.isEmpty {
                        VStack(spacing: 8) {
                            Image(systemName: "bubble.left.and.bubble.right")
                                .font(.system(size: 28))
                                .foregroundColor(WorkspaceTheme.textSoft.opacity(0.75))
                            if selectedSessionForConversation == nil {
                                Text("Select a session")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(WorkspaceTheme.textMuted)
                            } else if !selectedSessionIsChat {
                                Text("This session is currently in Terminal mode.")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(WorkspaceTheme.textMuted)
                                Text("Use \"Switch to Chat\" to continue the shared conversation here.")
                                    .font(.caption)
                                    .foregroundColor(WorkspaceTheme.textSoft)
                                Button {
                                    switchSelectedSessionToChat()
                                } label: {
                                    if switchingToChat {
                                        ProgressView()
                                    } else {
                                        Text("Switch to Chat")
                                    }
                                }
                                .buttonStyle(.borderedProminent)
                                .tint(WorkspaceTheme.accent)
                                .disabled(switchingToChat)
                            } else {
                                Text("No messages yet")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(WorkspaceTheme.textMuted)
                                Text("Create or select a chat session from the left rail to start.")
                                    .font(.caption)
                                    .foregroundColor(WorkspaceTheme.textSoft)
                            }
                        }
                        .padding(.top, 30)
                        .frame(maxWidth: .infinity, alignment: .center)
                    } else {
                        ForEach(appState.chatMessages) { message in
                            ChatBubbleView(message: message)
                                .id(message.id)
                        }
                    }
                }
                .padding(16)
                .frame(maxWidth: 940)
                .frame(maxWidth: .infinity)
            }
            .background(
                LinearGradient(
                    colors: [WorkspaceTheme.surfaceStrong.opacity(0.35), WorkspaceTheme.surface.opacity(0.88)],
                    startPoint: .top,
                    endPoint: .bottom
                )
            )
            .onChange(of: appState.chatMessages.count) { _ in
                scrollToBottom(proxy)
            }
            .onChange(of: appState.selectedChatSessionID) { _ in
                permissionExpanded = false
                localErrorText = nil
                if !selectedSessionIsChat { dismissKeyboard() }
                scrollToBottom(proxy)
            }
            .onChange(of: appState.selectedSessionID) { _ in
                permissionExpanded = false
                localErrorText = nil
                if !selectedSessionIsChat { dismissKeyboard() }
                scrollToBottom(proxy)
            }
            .onTapGesture {
                dismissKeyboard()
            }
        }
    }

    private var attachmentList: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(Array(attachments.enumerated()), id: \.element.id) { index, item in
                    ZStack(alignment: .topTrailing) {
                        ChatBinaryImage(data: item.rawData)
                            .frame(width: 74, height: 74)
                            .clipped()
                            .clipShape(RoundedRectangle(cornerRadius: 8))
                            .overlay(RoundedRectangle(cornerRadius: 8).stroke(WorkspaceTheme.border.opacity(0.8), lineWidth: 1))
                        Button {
                            attachments.remove(at: index)
                        } label: {
                            Image(systemName: "xmark.circle.fill")
                                .font(.system(size: 16))
                                .foregroundColor(.white)
                                .background(Circle().fill(Color.black.opacity(0.65)))
                        }
                        .buttonStyle(.plain)
                        .offset(x: 4, y: -4)
                    }
                    .frame(width: 78, height: 78)
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
        }
        .background(WorkspaceTheme.surfaceStrong.opacity(0.58))
    }

    private var inputBar: some View {
        VStack(spacing: 0) {
            HStack(alignment: .bottom, spacing: 10) {
                attachButton
                composerActionButton(icon: "doc.on.clipboard") {
                    addClipboardImage()
                }
                .disabled(attachments.count >= ChatAttachmentCodec.maxItemsPerMessage || !selectedSessionIsChat)

                TextEditor(text: $draftText)
                    .frame(height: composerTextHeight)
                    .font(.system(size: 14))
                    .foregroundColor(WorkspaceTheme.text)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 4)
                    .scrollContentBackground(.hidden)
                    .background(WorkspaceTheme.surface)
                    .overlay(
                        RoundedRectangle(cornerRadius: 10)
                            .stroke(WorkspaceTheme.border.opacity(0.9), lineWidth: 1)
                    )
                    .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                    .disabled(!selectedSessionIsChat)
                    #if os(iOS)
                    .focused($composerFocused)
                    #endif

                Button {
                    sendMessage()
                } label: {
                    Image(systemName: "arrow.up")
                        .font(.system(size: 12, weight: .bold))
                        .foregroundColor(.white)
                        .frame(width: 32, height: 32)
                        .background(Circle().fill(WorkspaceTheme.accent))
                }
                .buttonStyle(.plain)
                .disabled(sendDisabled)
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 10)
            .background(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .fill(WorkspaceTheme.surface)
                    .overlay(
                        RoundedRectangle(cornerRadius: 12, style: .continuous)
                            .stroke(WorkspaceTheme.border.opacity(0.92), lineWidth: 1)
                    )
            )
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
        }
        .background(WorkspaceTheme.surfaceStrong.opacity(0.96))
        .overlay(alignment: .top) {
            Rectangle()
                .fill(WorkspaceTheme.border.opacity(0.65))
                .frame(height: 1)
        }
    }

    @ViewBuilder
    private var attachButton: some View {
        #if os(iOS)
        PhotosPicker(selection: $pickerItems, maxSelectionCount: remainingSlots, matching: .images) {
            Image(systemName: "photo")
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(WorkspaceTheme.textMuted)
                .frame(width: 30, height: 30)
                .background(
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .fill(WorkspaceTheme.surfaceStrong)
                        .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(WorkspaceTheme.border, lineWidth: 1))
                )
        }
        .buttonStyle(.plain)
        .disabled(remainingSlots <= 0 || !selectedSessionIsChat)
        .onChange(of: pickerItems) { newItems in
            Task { await loadPickerItems(newItems) }
        }
        #else
        composerActionButton(icon: "photo") {
            showImageImporter = true
        }
        .disabled(remainingSlots <= 0)
        #endif
    }

    private var composerTextHeight: CGFloat {
        #if os(macOS)
        return 34
        #else
        return 44
        #endif
    }

    private func composerActionButton(icon: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: icon)
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(WorkspaceTheme.textMuted)
                .frame(width: 30, height: 30)
                .background(
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .fill(WorkspaceTheme.surfaceStrong)
                        .overlay(
                            RoundedRectangle(cornerRadius: 8, style: .continuous)
                                .stroke(WorkspaceTheme.border.opacity(0.95), lineWidth: 1)
                        )
                )
        }
        .buttonStyle(.plain)
    }

    private var sendDisabled: Bool {
        !selectedSessionIsChat ||
        (draftText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && attachments.isEmpty)
    }

    private var runStateColor: Color {
        switch appState.chatRunState {
        case .idle:
            return .clear
        case .running:
            return WorkspaceTheme.accent
        case .slow:
            return WorkspaceTheme.warning
        case .error:
            return WorkspaceTheme.danger
        }
    }

    private var remainingSlots: Int {
        max(0, ChatAttachmentCodec.maxItemsPerMessage - attachments.count)
    }

    private func scrollToBottom(_ proxy: ScrollViewProxy) {
        guard let last = appState.chatMessages.last else { return }
        DispatchQueue.main.async {
            withAnimation(.easeOut(duration: 0.2)) {
                proxy.scrollTo(last.id, anchor: .bottom)
            }
        }
    }

    private func copySessionID() {
        let value = selectedSessionIDForConversation ?? ""
        guard !value.isEmpty else { return }
        #if os(iOS)
        UIPasteboard.general.string = value
        #elseif os(macOS)
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(value, forType: .string)
        #endif
        copiedSessionID = true
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.4) { copiedSessionID = false }
    }

    private func dismissKeyboard() {
        #if os(iOS)
        composerFocused = false
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
        #endif
    }

    private func sendMessage() {
        localErrorText = nil
        guard selectedSessionIsChat else {
            localErrorText = "This session is currently in Terminal mode. Switch to Chat first."
            return
        }
        if appState.sendChatMessage(text: draftText, attachments: attachments) {
            draftText = ""
            attachments = []
        } else if case .error(let reason) = appState.chatRunState {
            localErrorText = reason
        }
    }

    private func switchSelectedSessionToChat() {
        guard let sessionID = selectedSessionIDForConversation, !sessionID.isEmpty else { return }
        localErrorText = nil
        switchingToChat = true
        Task {
            do {
                try await appState.switchSessionToChat(sessionID)
            } catch {
                await MainActor.run { localErrorText = error.localizedDescription }
            }
            await MainActor.run { switchingToChat = false }
        }
    }

    private func applyPermissionSettings() {
        guard let sessionID = appState.selectedChatSessionID, !sessionID.isEmpty else { return }
        localErrorText = nil
        applyingPermissions = true
        Task {
            do {
                try await appState.applyChatPermissions(
                    sessionID: sessionID,
                    permissionMode: permissionMode,
                    allowedTools: permissionAllowedTools,
                    disallowedTools: permissionDisallowedTools
                )
                await MainActor.run {
                    permissionExpanded = false
                }
            } catch {
                await MainActor.run { localErrorText = error.localizedDescription }
            }
            await MainActor.run { applyingPermissions = false }
        }
    }

    #if os(iOS)
    private func loadPickerItems(_ items: [PhotosPickerItem]) async {
        guard !items.isEmpty else { return }
        localErrorText = nil
        let allowed = min(remainingSlots, items.count)
        for item in items.prefix(allowed) {
            do {
                guard let data = try await item.loadTransferable(type: Data.self) else { continue }
                let attachment = try ChatAttachmentCodec.makeAttachment(name: "screenshot", data: data)
                attachments.append(attachment)
            } catch {
                localErrorText = error.localizedDescription
            }
        }
        pickerItems = []
    }
    #endif

    #if os(macOS)
    private func handleFileImport(result: Result<[URL], Error>) {
        guard case .success(let urls) = result else { return }
        localErrorText = nil
        let allowed = min(remainingSlots, urls.count)
        for url in urls.prefix(allowed) {
            do {
                let data = try Data(contentsOf: url)
                let attachment = try ChatAttachmentCodec.makeAttachment(
                    name: url.lastPathComponent,
                    data: data,
                    mediaTypeHint: UTType(filenameExtension: url.pathExtension)?.preferredMIMEType
                )
                attachments.append(attachment)
            } catch {
                localErrorText = error.localizedDescription
            }
        }
    }
    #endif

    private func addClipboardImage() {
        localErrorText = nil
        guard remainingSlots > 0 else {
            localErrorText = "Only \(ChatAttachmentCodec.maxItemsPerMessage) screenshots per message"
            return
        }
        do {
            guard let payload = try clipboardImagePayload() else {
                localErrorText = "Clipboard has no image"
                return
            }
            let item = try ChatAttachmentCodec.makeAttachment(name: payload.name, data: payload.data, mediaTypeHint: payload.mediaType)
            attachments.append(item)
        } catch {
            localErrorText = error.localizedDescription
        }
    }

    private func clipboardImagePayload() throws -> (name: String, data: Data, mediaType: String?)? {
        #if os(iOS)
        guard let image = UIPasteboard.general.image else { return nil }
        if let png = image.pngData() {
            return ("clipboard.png", png, "image/png")
        }
        if let jpg = image.jpegData(compressionQuality: 0.95) {
            return ("clipboard.jpg", jpg, "image/jpeg")
        }
        return nil
        #elseif os(macOS)
        let pb = NSPasteboard.general
        if let png = pb.data(forType: .png) {
            return ("clipboard.png", png, "image/png")
        }
        if let tiff = pb.data(forType: .tiff), let image = NSImage(data: tiff),
           let rep = NSBitmapImageRep(data: image.tiffRepresentation ?? Data()),
           let png = rep.representation(using: .png, properties: [:]) {
            return ("clipboard.png", png, "image/png")
        }
        return nil
        #else
        return nil
        #endif
    }
}

private struct ChatBubbleView: View {
    let message: ChatMessage

    private var isUser: Bool { message.isUser }

    var body: some View {
        VStack(alignment: isUser ? .trailing : .leading, spacing: 6) {
            VStack(alignment: .leading, spacing: 6) {
                if !displayText.isEmpty {
                    MarkdownBlockView(source: displayText, isUser: isUser)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                if !imageParts.isEmpty {
                    ForEach(Array(imageParts.enumerated()), id: \.offset) { _, data in
                        ChatBinaryImage(data: data)
                            .frame(maxWidth: 320, maxHeight: 280)
                            .clipShape(RoundedRectangle(cornerRadius: 8))
                            .overlay(RoundedRectangle(cornerRadius: 8).stroke(WorkspaceTheme.border.opacity(0.48), lineWidth: 1))
                    }
                }
                if !operations.isEmpty {
                    VStack(alignment: .leading, spacing: 3) {
                        Text("Intermediate steps")
                            .font(.caption2)
                            .foregroundColor(isUser ? Color.white.opacity(0.72) : WorkspaceTheme.textMuted)
                            .textCase(.uppercase)
                        ForEach(Array(operations.enumerated()), id: \.offset) { index, text in
                            Text("\(index + 1). \(text)")
                                .font(.caption)
                                .foregroundColor(isUser ? Color.white.opacity(0.82) : WorkspaceTheme.textMuted)
                        }
                    }
                    .padding(.top, 4)
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 9)
            .background(bubbleBackground)
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .stroke(isUser ? Color.white.opacity(0.08) : WorkspaceTheme.border.opacity(0.7), lineWidth: 1)
            )
            .contextMenu {
                Button {
                    copyToClipboard(displayText)
                } label: {
                    Label("Copy", systemImage: "doc.on.doc")
                }
            }

            Text(timestamp)
                .font(.caption2)
                .foregroundColor(WorkspaceTheme.textSoft)
        }
        .frame(maxWidth: .infinity, alignment: isUser ? .trailing : .leading)
        .padding(.horizontal, 2)
    }

    private var bubbleBackground: AnyShapeStyle {
        if isUser {
            return AnyShapeStyle(LinearGradient(
                colors: [
                    Color(red: 0.192, green: 0.373, blue: 0.447), // cc-web --accent
                    Color(red: 0.153, green: 0.310, blue: 0.384), // cc-web --accent-hover
                ],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            ))
        }
        return AnyShapeStyle(WorkspaceTheme.surface)
    }

    private var operations: [String] {
        message.meta?.operations?.filter { !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty } ?? []
    }

    private var displayText: String {
        if let parts = message.meta?.contentParts, !parts.isEmpty {
            let texts = parts.compactMap { part -> String? in
                guard part.type == "text" else { return nil }
                let value = part.text?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
                return value.isEmpty ? nil : value
            }
            if !texts.isEmpty {
                return texts.joined(separator: "\n\n")
            }
        }
        return message.content
    }

    private var imageParts: [Data] {
        guard let parts = message.meta?.contentParts, !parts.isEmpty else { return [] }
        return parts.compactMap { part in
            guard part.type == "image",
                  part.source?.type == "base64",
                  let encoded = part.source?.data else { return nil }
            return Data(base64Encoded: encoded)
        }
    }

    private var timestamp: String {
        let date = Date(timeIntervalSince1970: TimeInterval(message.tsMS) / 1000)
        return date.formatted(date: .omitted, time: .shortened)
    }

    private func copyToClipboard(_ text: String) {
        #if os(macOS)
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(text, forType: .string)
        #else
        UIPasteboard.general.string = text
        #endif
    }
}

// MARK: - Markdown block renderer

private struct MarkdownBlockView: View {
    let source: String
    let isUser: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(Array(blocks.enumerated()), id: \.offset) { _, block in
                renderBlock(block)
            }
        }
        .textSelection(.enabled)
    }

    // MARK: Block model

    private enum MDBlock {
        case paragraph(String)
        case codeBlock(lang: String, code: String)
        case heading(level: Int, text: String)
    }

    // MARK: Parser

    private var blocks: [MDBlock] {
        var result: [MDBlock] = []
        let lines = source.components(separatedBy: "\n")
        var i = 0
        var paraLines: [String] = []

        func flushParagraph() {
            let text = paraLines.joined(separator: "\n")
                .trimmingCharacters(in: CharacterSet.newlines)
            if !text.isEmpty { result.append(.paragraph(text)) }
            paraLines = []
        }

        while i < lines.count {
            let line = lines[i]

            // Fenced code block
            if line.hasPrefix("```") {
                flushParagraph()
                let lang = String(line.dropFirst(3)).trimmingCharacters(in: .whitespaces)
                i += 1
                var codeLines: [String] = []
                while i < lines.count && !lines[i].hasPrefix("```") {
                    codeLines.append(lines[i])
                    i += 1
                }
                if i < lines.count { i += 1 }
                result.append(.codeBlock(lang: lang, code: codeLines.joined(separator: "\n")))
                continue
            }

            // ATX heading
            if line.hasPrefix("#") {
                flushParagraph()
                var lvl = 0
                var rest = line[line.startIndex...]
                while rest.hasPrefix("#") { lvl += 1; rest = rest.dropFirst() }
                result.append(.heading(level: min(lvl, 6),
                                       text: String(rest).trimmingCharacters(in: .whitespaces)))
                i += 1
                continue
            }

            // Blank line = paragraph break
            if line.trimmingCharacters(in: .whitespaces).isEmpty {
                flushParagraph()
                i += 1
                continue
            }

            paraLines.append(line)
            i += 1
        }
        flushParagraph()
        return result
    }

    // MARK: Renderers

    @ViewBuilder
    private func renderBlock(_ block: MDBlock) -> some View {
        switch block {
        case .paragraph(let text):
            inlineMarkdown(text)
                .font(.system(size: 14))
                .foregroundColor(isUser ? .white : WorkspaceTheme.text)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)

        case .codeBlock(_, let code):
            Text(code)
                .font(.system(size: 12, design: .monospaced))
                .foregroundColor(isUser ? Color.white.opacity(0.9) : WorkspaceTheme.text)
                .padding(10)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(
                    RoundedRectangle(cornerRadius: 8)
                        .fill(Color.black.opacity(isUser ? 0.25 : 0.12))
                        .overlay(
                            RoundedRectangle(cornerRadius: 8)
                                .stroke(WorkspaceTheme.border.opacity(0.5), lineWidth: 1)
                        )
                )

        case .heading(let level, let text):
            inlineMarkdown(text)
                .font(headingFont(level))
                .foregroundColor(isUser ? .white : WorkspaceTheme.text)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func headingFont(_ level: Int) -> Font {
        switch level {
        case 1: return .system(size: 20, weight: .bold)
        case 2: return .system(size: 17, weight: .bold)
        case 3: return .system(size: 15, weight: .semibold)
        default: return .system(size: 14, weight: .semibold)
        }
    }

    @ViewBuilder
    private func inlineMarkdown(_ text: String) -> some View {
        if let attributed = try? AttributedString(
            markdown: text,
            options: AttributedString.MarkdownParsingOptions(
                interpretedSyntax: .inlineOnlyPreservingWhitespace,
                failurePolicy: .returnPartiallyParsedIfPossible
            )
        ) {
            Text(attributed)
        } else {
            Text(text)
        }
    }
}

private struct ChatBinaryImage: View {
    let data: Data

    var body: some View {
        #if os(iOS)
        if let uiImage = UIImage(data: data) {
            Image(uiImage: uiImage)
                .resizable()
                .scaledToFit()
        }
        #elseif os(macOS)
        if let nsImage = NSImage(data: data) {
            Image(nsImage: nsImage)
                .resizable()
                .scaledToFit()
        }
        #endif
    }
}
