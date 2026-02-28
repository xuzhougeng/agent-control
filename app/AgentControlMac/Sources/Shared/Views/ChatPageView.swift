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
        HStack(spacing: 0) {
            ChatSessionPanelView()
                .frame(minWidth: 280, idealWidth: 320, maxWidth: 360)
            Divider()
            ChatConversationView()
        }
    }
}

struct ChatSessionPanelView: View {
    @EnvironmentObject var appState: AppState
    @State private var cwd = ""
    @State private var envString = ""
    @State private var isCreating = false
    @State private var errorText: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Chat")
                    .font(.headline)
                Spacer()
                Button {
                    Task {
                        await appState.fetchServers()
                        await appState.fetchSessions()
                    }
                } label: {
                    Image(systemName: "arrow.clockwise")
                }
                .buttonStyle(.plain)
                .help("Refresh")
            }

            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    serversSection
                    newChatSection
                    sessionsSection
                }
            }
            if let errorText, !errorText.isEmpty {
                Text(errorText)
                    .font(.caption)
                    .foregroundColor(.red)
            }
        }
        .padding(12)
    }

    private var serversSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Servers")
                .font(.caption)
                .foregroundColor(.secondary)
            if appState.servers.isEmpty {
                Text("No servers")
                    .foregroundColor(.secondary)
                    .font(.caption)
            } else {
                ForEach(appState.servers) { server in
                    Button {
                        appState.selectedServerID = server.serverID
                        Task { await appState.fetchSessions() }
                    } label: {
                        HStack {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(server.serverID)
                                    .font(.system(.subheadline, design: .monospaced))
                                if !server.hostname.isEmpty {
                                    Text(server.hostname)
                                        .font(.caption2)
                                        .foregroundColor(.secondary)
                                }
                            }
                            Spacer()
                            StatusBadge(label: server.status, isOnline: server.isOnline)
                        }
                        .padding(8)
                        .background(
                            RoundedRectangle(cornerRadius: 8)
                                .fill(server.serverID == appState.selectedServerID ? Color.accentColor.opacity(0.12) : Color.clear)
                        )
                    }
                    .buttonStyle(.plain)
                }
            }
        }
    }

    private var newChatSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("New Chat")
                .font(.caption)
                .foregroundColor(.secondary)
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
                .disabled(isCreating || cwd.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || appState.selectedServerID == nil)
            }
        }
    }

    private var sessionsSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Chat Sessions")
                .font(.caption)
                .foregroundColor(.secondary)
            if appState.chatSessions.isEmpty {
                Text("No chat sessions")
                    .foregroundColor(.secondary)
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
                                    StatusBadge(label: session.status, isOnline: session.isRunning)
                                }
                                Text(session.cwd)
                                    .font(.caption)
                                    .foregroundColor(.secondary)
                                    .lineLimit(1)
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(8)
                            .background(
                                RoundedRectangle(cornerRadius: 8)
                                    .fill(session.sessionID == appState.selectedChatSessionID ? Color.accentColor.opacity(0.12) : Color.clear)
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

struct ChatConversationView: View {
    @EnvironmentObject var appState: AppState
    @State private var draftText = ""
    @State private var attachments: [ChatAttachment] = []
    @State private var localErrorText: String?
    @State private var copiedSessionID = false

    #if os(iOS)
    @State private var pickerItems: [PhotosPickerItem] = []
    #elseif os(macOS)
    @State private var showImageImporter = false
    #endif

    var body: some View {
        VStack(spacing: 0) {
            sessionInfoBar
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
                    .foregroundColor(.red)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 6)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            inputBar
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
            Text(appState.selectedChatSessionID.map { "Chat: \(String($0.prefix(8)))" } ?? "Chat: (none)")
                .font(.system(size: 11, design: .monospaced))
                .foregroundColor(.secondary)
                .lineLimit(1)
            Spacer()
            Button(copiedSessionID ? "Copied" : "Copy") {
                copySessionID()
            }
            .buttonStyle(.bordered)
            .controlSize(.small)
            .disabled(appState.selectedChatSessionID == nil)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 7)
        .background(Color.secondary.opacity(0.08))
    }

    private var runStateBar: some View {
        Text(appState.chatRunState.text)
            .font(.caption)
            .foregroundColor(runStateColor)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .background(runStateColor.opacity(0.1))
    }

    private var messageList: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 10) {
                    if appState.chatMessages.isEmpty {
                        Text("No messages yet")
                            .font(.caption)
                            .foregroundColor(.secondary)
                            .padding(.top, 20)
                            .frame(maxWidth: .infinity, alignment: .center)
                    } else {
                        ForEach(appState.chatMessages) { message in
                            ChatBubbleView(message: message)
                                .id(message.id)
                        }
                    }
                }
                .padding(12)
            }
            .onChange(of: appState.chatMessages.count) { _ in
                scrollToBottom(proxy)
            }
            .onChange(of: appState.selectedChatSessionID) { _ in
                scrollToBottom(proxy)
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
                            .overlay(RoundedRectangle(cornerRadius: 8).stroke(Color.secondary.opacity(0.3), lineWidth: 1))
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
        .background(Color.secondary.opacity(0.05))
    }

    private var inputBar: some View {
        VStack(spacing: 8) {
            HStack(alignment: .bottom, spacing: 8) {
                attachButton
                Button("Paste") {
                    addClipboardImage()
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .disabled(attachments.count >= ChatAttachmentCodec.maxItemsPerMessage)
                TextEditor(text: $draftText)
                    .frame(minHeight: 38, maxHeight: 120)
                    .padding(5)
                    .overlay(
                        RoundedRectangle(cornerRadius: 8)
                            .stroke(Color.secondary.opacity(0.35), lineWidth: 1)
                    )
                Button("Send") {
                    sendMessage()
                }
                .buttonStyle(.borderedProminent)
                .disabled(sendDisabled)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
        }
        .background(Color.secondary.opacity(0.08))
    }

    @ViewBuilder
    private var attachButton: some View {
        #if os(iOS)
        PhotosPicker(selection: $pickerItems, maxSelectionCount: remainingSlots, matching: .images) {
            Text("Image")
        }
        .buttonStyle(.bordered)
        .controlSize(.small)
        .disabled(remainingSlots <= 0)
        .onChange(of: pickerItems) { newItems in
            Task { await loadPickerItems(newItems) }
        }
        #else
        Button("Image") {
            showImageImporter = true
        }
        .buttonStyle(.bordered)
        .controlSize(.small)
        .disabled(remainingSlots <= 0)
        #endif
    }

    private var sendDisabled: Bool {
        appState.selectedChatSessionID == nil ||
        (draftText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && attachments.isEmpty)
    }

    private var runStateColor: Color {
        switch appState.chatRunState {
        case .idle:
            return .clear
        case .running:
            return .blue
        case .slow:
            return .orange
        case .error:
            return .red
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
        let value = appState.selectedChatSessionID ?? ""
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

    private func sendMessage() {
        localErrorText = nil
        if appState.sendChatMessage(text: draftText, attachments: attachments) {
            draftText = ""
            attachments = []
        } else if case .error(let reason) = appState.chatRunState {
            localErrorText = reason
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
                    markdownText(displayText)
                        .font(.system(size: 14))
                        .foregroundColor(isUser ? .white : .primary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                if !imageParts.isEmpty {
                    ForEach(Array(imageParts.enumerated()), id: \.offset) { _, data in
                        ChatBinaryImage(data: data)
                            .frame(maxWidth: 320, maxHeight: 280)
                            .clipShape(RoundedRectangle(cornerRadius: 8))
                            .overlay(RoundedRectangle(cornerRadius: 8).stroke(Color.secondary.opacity(0.25), lineWidth: 1))
                    }
                }
                if !operations.isEmpty {
                    VStack(alignment: .leading, spacing: 3) {
                        Text("Intermediate steps")
                            .font(.caption2)
                            .foregroundColor(.secondary)
                            .textCase(.uppercase)
                        ForEach(Array(operations.enumerated()), id: \.offset) { index, text in
                            Text("\(index + 1). \(text)")
                                .font(.caption)
                                .foregroundColor(.secondary)
                        }
                    }
                    .padding(.top, 4)
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 9)
            .background(isUser ? Color.accentColor : Color.secondary.opacity(0.12))
            .clipShape(RoundedRectangle(cornerRadius: 12))

            Text(timestamp)
                .font(.caption2)
                .foregroundColor(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: isUser ? .trailing : .leading)
        .padding(.horizontal, 2)
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

    @ViewBuilder
    private func markdownText(_ raw: String) -> some View {
        if let attributed = try? AttributedString(
            markdown: raw,
            options: AttributedString.MarkdownParsingOptions(
                interpretedSyntax: .full,
                failurePolicy: .returnPartiallyParsedIfPossible
            )
        ) {
            Text(attributed)
        } else {
            Text(raw)
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
