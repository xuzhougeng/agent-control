import Foundation
import Combine

enum AppDetailPage: String, CaseIterable {
    case terminal
    case chat
}

enum ChatRunState: Equatable {
    case idle
    case running(pending: Int)
    case slow(pending: Int)
    case error(String)

    var text: String {
        switch self {
        case .idle:
            return ""
        case .running(let pending):
            if pending > 1 { return "Running... (\(pending) requests pending)" }
            return "Running... waiting for assistant response"
        case .slow:
            return "Still running... this is taking longer than usual"
        case .error(let reason):
            return "Execution failed: \(reason)"
        }
    }
}

struct ChatAttachment: Identifiable {
    let id = UUID()
    let name: String
    let mediaType: String
    let base64Data: String
    let rawData: Data
}

@MainActor
final class AppState: ObservableObject {
    // MARK: - Published state
    @Published var servers: [Server] = []
    @Published var sessions: [Session] = []
    @Published var approvals: [String: SessionEvent] = [:]  // keyed by eventID
    @Published var notifications: [String: NotificationEvent] = [:]  // keyed by notificationID
    @Published var transientNotification: NotificationEvent?
    @Published var selectedServerID: String?
    @Published var selectedSessionID: String?
    @Published var selectedChatSessionID: String?
    @Published var selectedPage: AppDetailPage = .terminal
    @Published var chatMessages: [ChatMessage] = []
    @Published var chatRunState: ChatRunState = .idle
    @Published var wsConnected = false
    @Published var showNewSessionSheet = false
    @Published var connectionHint: String?

    // MARK: - Subsystems
    let apiClient = APIClient()
    let wsClient = WSClient()
    let terminalBridge = TerminalBridge()

    // Debounce: coalesce rapid session_update WS messages into one REST fetch
    private var sessionRefreshTask: DispatchWorkItem?
    /// Prevent multiple start() from reconnecting (e.g. iOS onAppear firing repeatedly).
    private var didStart = false
    /// Track real background->foreground transitions on iOS.
    private var wasBackgrounded = false
    /// Some configs are valid syntactically but cannot be used on current platform.
    private var shouldAutoConnect = true
    /// Startup race guard: replay resize until PTY is fully ready.
    private var resizeReplayTask: Task<Void, Never>?
    private var chatSlowTask: Task<Void, Never>?
    private var transientNotificationTask: Task<Void, Never>?
    private var chatPendingTurns = 0

    private let slowResponseDelayNS: UInt64 = 12_000_000_000

    var pendingApprovals: [SessionEvent] {
        approvals.values.filter { !$0.resolved }.sorted { $0.tsMS > $1.tsMS }
    }

    var recentNotifications: [NotificationEvent] {
        notifications.values.sorted { $0.tsMS > $1.tsMS }
    }

    var ptySessions: [Session] {
        sessions.filter { $0.isPTY }
    }

    var chatSessions: [Session] {
        sessions.filter { $0.isChat }
    }

    // MARK: - Init

    nonisolated init() {}

    func start() {
        loadConfig()
        wsClient.onMessage = { [weak self] msg in self?.handleWSMessage(msg) }
        wsClient.onConnectionChange = { [weak self] connected in
            guard let self else { return }
            self.wsConnected = connected
            if connected {
                self.onConnectedReattach()
            }
        }
        guard !didStart else { return }
        didStart = true
        guard shouldAutoConnect else {
            wsConnected = false
            return
        }
        wsClient.connect()
        Task {
            await fetchServers()
            await fetchSessions()
        }
    }

    /// Call when the app moves to background (iOS) to gracefully disconnect WS.
    func pause() {
        wasBackgrounded = true
        resizeReplayTask?.cancel()
        transientNotificationTask?.cancel()
        transientNotification = nil
        wsClient.disconnect(reconnect: false)
    }

    /// Call when the app returns to foreground (iOS) to reconnect WS.
    func resume() {
        guard wasBackgrounded else { return }
        wasBackgrounded = false
        guard shouldAutoConnect else {
            wsConnected = false
            return
        }
        if !wsConnected { wsClient.connect() }
        Task {
            await fetchServers()
            await fetchSessions()
            if let sid = selectedChatSessionID {
                await fetchChatHistory(sessionID: sid)
            }
        }
    }

    // MARK: - Config

    private func loadConfig() {
        let baseURL = UserDefaults.standard.string(forKey: "baseURL") ?? "http://127.0.0.1:18080"
        let token = KeychainHelper.load(key: "ui_token") ?? "admin-dev-token"
        let skipTLSVerify = UserDefaults.standard.bool(forKey: "skipTLSVerify")
        apiClient.configure(baseURL: baseURL, token: token, skipTLSVerify: skipTLSVerify)
        wsClient.configure(baseURL: baseURL, token: token, skipTLSVerify: skipTLSVerify)
        applyConnectionPolicy(baseURL: baseURL)
    }

    private static let loopbackHosts: Set<String> = ["127.0.0.1", "localhost", "::1"]

    private static func shouldBlockAutoConnect(baseURL: String) -> Bool {
        guard let host = URLComponents(string: baseURL)?.host?.lowercased(),
              loopbackHosts.contains(host) else {
            return false
        }
        #if os(iOS)
        #if targetEnvironment(simulator)
        return false
        #else
        return true
        #endif
        #else
        return false
        #endif
    }

    private func applyConnectionPolicy(baseURL: String) {
        shouldAutoConnect = !Self.shouldBlockAutoConnect(baseURL: baseURL)
        if shouldAutoConnect {
            connectionHint = nil
        } else {
            connectionHint = "Current Base URL points to localhost. On iPhone/iPad, set Base URL to your Mac/server address (for example http://192.168.x.x:18080)."
        }
    }

    static func isValidBaseURL(_ raw: String) -> Bool {
        let value = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let components = URLComponents(string: value),
              let scheme = components.scheme?.lowercased(),
              ["http", "https"].contains(scheme),
              let host = components.host, !host.isEmpty else {
            return false
        }
        if let port = components.port, !(1...65535).contains(port) {
            return false
        }
        // Keep base URL simple to avoid unexpected path joining issues.
        return components.path.isEmpty || components.path == "/"
    }

    func saveConfig(baseURL: String, token: String, skipTLSVerify: Bool = false) {
        guard Self.isValidBaseURL(baseURL) else { return }
        UserDefaults.standard.set(baseURL, forKey: "baseURL")
        UserDefaults.standard.set(skipTLSVerify, forKey: "skipTLSVerify")
        KeychainHelper.save(key: "ui_token", value: token)
        apiClient.configure(baseURL: baseURL, token: token, skipTLSVerify: skipTLSVerify)
        wsClient.configure(baseURL: baseURL, token: token, skipTLSVerify: skipTLSVerify)
        applyConnectionPolicy(baseURL: baseURL)
        approvals.removeAll()
        notifications.removeAll()
        transientNotification = nil
        transientNotificationTask?.cancel()
        wsClient.disconnect()
        guard shouldAutoConnect else {
            wsConnected = false
            return
        }
        wsClient.connect()
        Task {
            await fetchServers()
            await fetchSessions()
        }
    }

    // MARK: - REST

    func fetchServers() async {
        do {
            servers = try await apiClient.fetchServers()
            if selectedServerID == nil, let first = servers.first {
                selectedServerID = first.serverID
            }
        } catch { print("[api] fetchServers: \(error)") }
    }

    func fetchSessions() async {
        do {
            sessions = try await apiClient.fetchSessions(serverID: selectedServerID)
            normalizeSessionSelection()
        } catch { print("[api] fetchSessions: \(error)") }
    }

    func createSession(cwd: String, sessionID: String?, env: [String: String]) async {
        guard let serverID = selectedServerID else { return }
        do {
            let sess = try await apiClient.createSession(
                serverID: serverID,
                cwd: cwd,
                sessionID: sessionID,
                env: env,
                sessionType: .pty,
                cols: terminalBridge.currentCols,
                rows: terminalBridge.currentRows
            )
            await fetchSessions()
            openSession(sess)
        } catch { print("[api] createSession: \(error)") }
    }

    func createChatSession(cwd: String, sessionID: String?, env: [String: String]) async {
        guard let serverID = selectedServerID else { return }
        do {
            let session = try await apiClient.createSession(
                serverID: serverID,
                cwd: cwd,
                sessionID: sessionID,
                env: env,
                sessionType: .chat
            )
            await fetchSessions()
            await attachChatSession(session.sessionID)
        } catch { print("[api] createChatSession: \(error)") }
    }

    func stopSession(_ sessionID: String) async {
        do {
            try await apiClient.stopSession(sessionID)
            await fetchSessions()
        } catch { print("[api] stopSession: \(error)") }
    }

    func deleteSession(_ sessionID: String) async {
        do {
            try await apiClient.deleteSession(sessionID)
            if selectedSessionID == sessionID {
                selectedSessionID = nil
                terminalBridge.clear()
            }
            if selectedChatSessionID == sessionID {
                resetChatSelection()
            }
            approvals = approvals.filter { $0.value.sessionID != sessionID }
            notifications = notifications.filter { $0.value.sessionID != sessionID }
            await fetchSessions()
        } catch { print("[api] deleteSession: \(error)") }
    }

    func fetchChatHistory(sessionID: String) async {
        guard !sessionID.isEmpty else { return }
        do {
            let messages = try await apiClient.fetchChatHistory(sessionID)
            guard selectedChatSessionID == sessionID else { return }
            chatMessages = messages
        } catch {
            print("[api] fetchChatHistory: \(error)")
        }
    }

    /// Coalesce rapid session_update pushes: wait 1s of silence before firing REST fetch.
    private func debouncedFetchSessions() {
        sessionRefreshTask?.cancel()
        let work = DispatchWorkItem { [weak self] in
            Task { @MainActor [weak self] in
                await self?.fetchSessions()
            }
        }
        sessionRefreshTask = work
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.0, execute: work)
    }

    // MARK: - Navigation helpers

    func openSession(_ session: Session) {
        if session.isChat {
            selectedPage = .chat
            Task { await attachChatSession(session.sessionID) }
            return
        }
        selectedPage = .terminal
        attachSession(session.sessionID)
    }

    // MARK: - WS actions

    func attachSession(_ sessionID: String) {
        selectedSessionID = sessionID
        selectedPage = .terminal
        terminalBridge.clear()
        terminalBridge.prepareForAttach()
        wsClient.sendAttach(sessionID: sessionID)
        sendResize(cols: terminalBridge.currentCols, rows: terminalBridge.currentRows)
        scheduleResizeReplay(sessionID: sessionID)
    }

    func attachChatSession(_ sessionID: String) async {
        guard !sessionID.isEmpty else { return }
        selectedChatSessionID = sessionID
        selectedPage = .chat
        chatMessages = []
        chatPendingTurns = 0
        cancelSlowStateTransition()
        chatRunState = .idle
        _ = wsClient.sendAttach(sessionID: sessionID)
        await fetchChatHistory(sessionID: sessionID)
    }

    /// Keep Chat page active while selecting a non-chat session (cc-web behavior).
    func selectSessionForChatView(_ sessionID: String) {
        guard !sessionID.isEmpty else { return }
        selectedSessionID = sessionID
        selectedChatSessionID = nil
        selectedPage = .chat
        chatMessages = []
        chatPendingTurns = 0
        cancelSlowStateTransition()
        chatRunState = .idle
    }

    func switchSessionToChat(_ sessionID: String) async throws {
        guard !sessionID.isEmpty else { return }
        _ = try await apiClient.switchSession(sessionID, to: .chat, env: [:])
        await fetchSessions()
        await attachChatSession(sessionID)
    }

    func switchSessionToPTY(_ sessionID: String) async throws {
        guard !sessionID.isEmpty else { return }
        _ = try await apiClient.switchSession(
            sessionID,
            to: .pty,
            env: [:],
            cols: terminalBridge.currentCols,
            rows: terminalBridge.currentRows
        )
        await fetchSessions()
        attachSession(sessionID)
    }

    func applyChatPermissions(
        sessionID: String,
        permissionMode: String,
        allowedTools: String,
        disallowedTools: String
    ) async throws {
        guard !sessionID.isEmpty else { return }
        var env: [String: String] = [:]
        let mode = permissionMode.trimmingCharacters(in: .whitespacesAndNewlines)
        let allowed = allowedTools.trimmingCharacters(in: .whitespacesAndNewlines)
        let disallowed = disallowedTools.trimmingCharacters(in: .whitespacesAndNewlines)
        if !mode.isEmpty { env["CC_CLAUDE_PERMISSION_MODE"] = mode }
        if !allowed.isEmpty { env["CC_CLAUDE_ALLOWED_TOOLS"] = allowed }
        if !disallowed.isEmpty { env["CC_CLAUDE_DISALLOWED_TOOLS"] = disallowed }

        _ = try await apiClient.switchSession(sessionID, to: .chat, env: env)
        await fetchSessions()
        await attachChatSession(sessionID)
    }

    @discardableResult
    func sendChatMessage(text: String, attachments: [ChatAttachment]) -> Bool {
        guard let sid = selectedChatSessionID, !sid.isEmpty else {
            if let selected = selectedSessionID, !selected.isEmpty {
                chatRunState = .error("This session is currently in Terminal mode. Switch to Chat first.")
            } else {
                chatRunState = .error("select or create a chat session first")
            }
            return false
        }
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        var parts: [ChatContentPart] = []
        if !trimmed.isEmpty {
            parts.append(ChatContentPart(type: "text", text: trimmed, source: nil))
        }
        for entry in attachments {
            let source = ChatImageSource(type: "base64", mediaType: entry.mediaType, data: entry.base64Data)
            parts.append(ChatContentPart(type: "image", text: nil, source: source))
        }
        if parts.isEmpty { return false }
        guard wsConnected else {
            chatRunState = .error("WebSocket disconnected")
            return false
        }
        let sent = wsClient.sendChatIn(sessionID: sid, content: trimmed, contentParts: parts)
        if !sent {
            chatRunState = .error("WebSocket disconnected")
            return false
        }
        beginPendingChatTurn()
        return true
    }

    func sendTerminalInput(_ bytes: Data) {
        guard let sid = selectedSessionID else { return }
        _ = wsClient.sendTermIn(sessionID: sid, dataB64: bytes.base64EncodedString())
    }

    func sendResize(cols: Int, rows: Int) {
        guard let sid = selectedSessionID else { return }
        _ = wsClient.sendResize(sessionID: sid, cols: cols, rows: rows)
    }

    func sendAction(sessionID: String, kind: String) {
        _ = wsClient.sendAction(sessionID: sessionID, kind: kind)
    }

    private func scheduleResizeReplay(sessionID: String) {
        resizeReplayTask?.cancel()
        let retryDelaysNS: [UInt64] = [
            0,
            250_000_000,
            500_000_000,
            1_000_000_000,
            2_000_000_000,
        ]
        resizeReplayTask = Task { @MainActor [weak self] in
            guard let self else { return }
            for delay in retryDelaysNS {
                if delay > 0 {
                    try? await Task.sleep(nanoseconds: delay)
                }
                guard !Task.isCancelled else { return }
                guard self.selectedSessionID == sessionID else { return }
                self.sendResize(cols: self.terminalBridge.currentCols, rows: self.terminalBridge.currentRows)
            }
        }
    }

    private func onConnectedReattach() {
        if let sid = selectedSessionID, !sid.isEmpty {
            terminalBridge.clear()
            terminalBridge.prepareForAttach()
            wsClient.sendAttach(sessionID: sid)
            sendResize(cols: terminalBridge.currentCols, rows: terminalBridge.currentRows)
            scheduleResizeReplay(sessionID: sid)
        }
        if let chatID = selectedChatSessionID, !chatID.isEmpty {
            wsClient.sendAttach(sessionID: chatID)
        }
    }

    private func normalizeSessionSelection() {
        if let sid = selectedSessionID,
           !sessions.contains(where: { $0.sessionID == sid && $0.isPTY }) {
            selectedSessionID = nil
            terminalBridge.clear()
        }
        if let sid = selectedChatSessionID {
            if sessions.first(where: { $0.sessionID == sid && $0.isChat }) == nil {
                resetChatSelection()
            }
        }
    }

    private func resetChatSelection() {
        selectedChatSessionID = nil
        chatMessages = []
        chatPendingTurns = 0
        cancelSlowStateTransition()
        chatRunState = .idle
    }

    // MARK: - Chat run state

    private func beginPendingChatTurn() {
        chatPendingTurns += 1
        chatRunState = .running(pending: chatPendingTurns)
        if chatPendingTurns == 1 {
            scheduleSlowStateTransition()
        }
    }

    private func completePendingChatTurn() {
        guard chatPendingTurns > 0 else { return }
        chatPendingTurns -= 1
        if chatPendingTurns == 0 {
            cancelSlowStateTransition()
            chatRunState = .idle
            return
        }
        chatRunState = .running(pending: chatPendingTurns)
    }

    private func failPendingChatTurns(reason: String) {
        guard chatPendingTurns > 0 else { return }
        chatPendingTurns = 0
        cancelSlowStateTransition()
        chatRunState = .error(reason)
    }

    private func scheduleSlowStateTransition() {
        cancelSlowStateTransition()
        chatSlowTask = Task { @MainActor [weak self] in
            guard let self else { return }
            try? await Task.sleep(nanoseconds: slowResponseDelayNS)
            guard !Task.isCancelled else { return }
            guard self.chatPendingTurns > 0 else { return }
            self.chatRunState = .slow(pending: self.chatPendingTurns)
        }
    }

    private func cancelSlowStateTransition() {
        chatSlowTask?.cancel()
        chatSlowTask = nil
    }

    private func isProgressMessage(_ msg: ChatMessage) -> Bool {
        msg.meta?.source == "claude-stream-json" && msg.meta?.progress == true
    }

    // MARK: - WS message handler

    private func handleWSMessage(_ msg: WSMessage) {
        switch msg {
        case .termOut(let sessionID, let data, _):
            if sessionID == selectedSessionID {
                terminalBridge.feed(data)
            }

        case .chatMsg(let chatMsg):
            if chatMsg.sessionID == selectedChatSessionID {
                chatMessages.append(chatMsg)
                if chatMsg.isAssistant && !isProgressMessage(chatMsg) {
                    completePendingChatTurn()
                }
            }

        case .event(let event):
            if event.kind == "approval_needed" {
                approvals[event.eventID] = event
            }

        case .notification(let notification):
            handleNotification(notification)

        case .sessionUpdate(let update):
            // Update local session state inline to avoid REST round-trip for every WS push
            if let idx = sessions.firstIndex(where: { $0.sessionID == update.sessionID }) {
                let old = sessions[idx]
                let patched = Session(
                    sessionID: old.sessionID,
                    serverID: old.serverID,
                    sessionType: update.sessionType ?? old.sessionType,
                    cwd: old.cwd,
                    cmd: old.cmd,
                    envKeys: old.envKeys,
                    status: update.status.isEmpty ? old.status : update.status,
                    createdBy: old.createdBy,
                    createdAtMS: old.createdAtMS,
                    exitCode: update.exitCode ?? old.exitCode,
                    exitReason: update.exitReason ?? old.exitReason,
                    awaitingApproval: update.awaitingApproval,
                    pendingEventID: update.pendingEventID ?? old.pendingEventID
                )
                sessions[idx] = patched
            }
            if update.sessionID == selectedChatSessionID {
                if update.status == "error" || update.status == "exited" {
                    failPendingChatTurns(reason: update.exitReason ?? "session \(update.status)")
                }
            }
            if !update.awaitingApproval {
                for (key, ev) in approvals where ev.sessionID == update.sessionID && !ev.resolved {
                    approvals[key]?.resolved = true
                }
            }
            // Debounce: coalesce rapid updates into a single REST fetch
            debouncedFetchSessions()

        case .attachOK(let sessionID):
            if sessionID == selectedSessionID {
                scheduleResizeReplay(sessionID: sessionID)
            }

        case .error(let sessionID, let message):
            print("[ws] error session=\(sessionID): \(message)")
            if sessionID == selectedChatSessionID {
                failPendingChatTurns(reason: message)
            }
        }
    }

    private func handleNotification(_ notification: NotificationEvent) {
        let existed = notifications[notification.notificationID] != nil
        notifications[notification.notificationID] = notification

        if notifications.count > 100 {
            let keep = notifications.values
                .sorted { $0.tsMS > $1.tsMS }
                .prefix(100)
            notifications = Dictionary(uniqueKeysWithValues: keep.map { ($0.notificationID, $0) })
        }

        guard !existed else { return }
        guard isFreshNotification(notification) else { return }

        transientNotification = notification
        transientNotificationTask?.cancel()
        transientNotificationTask = Task { @MainActor [weak self] in
            guard let self else { return }
            try? await Task.sleep(nanoseconds: 6_000_000_000)
            guard !Task.isCancelled else { return }
            if self.transientNotification?.notificationID == notification.notificationID {
                self.transientNotification = nil
            }
        }
    }

    private func isFreshNotification(_ notification: NotificationEvent) -> Bool {
        let nowMS = Int64(Date().timeIntervalSince1970 * 1000)
        return notification.tsMS >= nowMS - 15_000
    }
}
