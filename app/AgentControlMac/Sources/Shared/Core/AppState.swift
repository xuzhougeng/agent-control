import Foundation
import Combine

@MainActor
final class AppState: ObservableObject {
    // MARK: - Published state
    @Published var servers: [Server] = []
    @Published var sessions: [Session] = []
    @Published var approvals: [String: SessionEvent] = [:]  // keyed by eventID
    @Published var selectedServerID: String?
    @Published var selectedSessionID: String?
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

    var pendingApprovals: [SessionEvent] {
        approvals.values.filter { !$0.resolved }.sorted { $0.tsMS > $1.tsMS }
    }

    // MARK: - Init

    nonisolated init() {}

    func start() {
        loadConfig()
        wsClient.onMessage = { [weak self] msg in self?.handleWSMessage(msg) }
        wsClient.onConnectionChange = { [weak self] connected in
            guard let self else { return }
            self.wsConnected = connected
            if connected, let sid = self.selectedSessionID {
                self.terminalBridge.clear()
                self.wsClient.sendAttach(sessionID: sid)
                self.sendResize(cols: self.terminalBridge.currentCols, rows: self.terminalBridge.currentRows)
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
        } catch { print("[api] fetchSessions: \(error)") }
    }

    func createSession(cwd: String, resumeID: String?, env: [String: String]) async {
        guard let serverID = selectedServerID else { return }
        do {
            let sess = try await apiClient.createSession(
                serverID: serverID, cwd: cwd, resumeID: resumeID, env: env,
                cols: terminalBridge.currentCols, rows: terminalBridge.currentRows
            )
            await fetchSessions()
            attachSession(sess.sessionID)
        } catch { print("[api] createSession: \(error)") }
    }

    func resumeSession(_ session: Session) async {
        guard !session.cwd.isEmpty, let rid = session.resumeID, !rid.isEmpty else { return }
        let serverID = session.serverID.isEmpty ? (selectedServerID ?? "") : session.serverID
        guard !serverID.isEmpty else { return }
        do {
            let sess = try await apiClient.createSession(
                serverID: serverID, cwd: session.cwd, resumeID: rid, env: [:],
                cols: terminalBridge.currentCols, rows: terminalBridge.currentRows
            )
            selectedServerID = serverID
            await fetchSessions()
            attachSession(sess.sessionID)
        } catch { print("[api] resumeSession: \(error)") }
    }

    func stopSession(_ sessionID: String) async {
        do {
            try await apiClient.stopSession(sessionID)
            await fetchSessions()
        } catch { print("[api] stopSession: \(error)") }
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

    // MARK: - WS actions

    func attachSession(_ sessionID: String) {
        selectedSessionID = sessionID
        terminalBridge.clear()
        wsClient.sendAttach(sessionID: sessionID)
        sendResize(cols: terminalBridge.currentCols, rows: terminalBridge.currentRows)
    }

    func sendTerminalInput(_ bytes: Data) {
        guard let sid = selectedSessionID else { return }
        wsClient.sendTermIn(sessionID: sid, dataB64: bytes.base64EncodedString())
    }

    func sendResize(cols: Int, rows: Int) {
        guard let sid = selectedSessionID else { return }
        wsClient.sendResize(sessionID: sid, cols: cols, rows: rows)
    }

    func sendAction(sessionID: String, kind: String) {
        wsClient.sendAction(sessionID: sessionID, kind: kind)
    }

    // MARK: - WS message handler

    private func handleWSMessage(_ msg: WSMessage) {
        switch msg {
        case .termOut(let sessionID, let data, _):
            if sessionID == selectedSessionID {
                terminalBridge.feed(data)
            }

        case .event(let event):
            if event.kind == "approval_needed" {
                approvals[event.eventID] = event
            }

        case .sessionUpdate(let update):
            // Update local session state inline to avoid REST round-trip for every WS push
            if let idx = sessions.firstIndex(where: { $0.sessionID == update.sessionID }) {
                // Decode a fresh copy with the updated fields
                let old = sessions[idx]
                let patched = Session(
                    sessionID: old.sessionID, serverID: old.serverID,
                    cwd: old.cwd, cmd: old.cmd,
                    resumeID: update.resumeID ?? old.resumeID,
                    envKeys: old.envKeys, status: update.status.isEmpty ? old.status : update.status,
                    createdBy: old.createdBy, createdAtMS: old.createdAtMS,
                    exitCode: update.exitCode ?? old.exitCode,
                    exitReason: update.exitReason ?? old.exitReason,
                    awaitingApproval: update.awaitingApproval,
                    pendingEventID: update.pendingEventID ?? old.pendingEventID
                )
                sessions[idx] = patched
            }
            if !update.awaitingApproval {
                for (key, ev) in approvals where ev.sessionID == update.sessionID && !ev.resolved {
                    approvals[key]?.resolved = true
                }
            }
            // Debounce: coalesce rapid updates into a single REST fetch
            debouncedFetchSessions()

        case .attachOK:
            break

        case .error(let sessionID, let message):
            print("[ws] error session=\(sessionID): \(message)")
        }
    }
}
