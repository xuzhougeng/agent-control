import Foundation

final class WSClient: NSObject, URLSessionWebSocketDelegate, URLSessionDelegate {
    private var baseURL = ""
    private var token = ""
    private var skipTLSVerify = false
    private var task: URLSessionWebSocketTask?
    private var urlSession: URLSession?
    private var shouldReconnect = true
    private var reconnectDelay: TimeInterval = 1.0

    var onMessage: ((WSMessage) -> Void)?
    var onConnectionChange: ((Bool) -> Void)?

    func configure(baseURL: String, token: String, skipTLSVerify: Bool = false) {
        self.baseURL = baseURL.hasSuffix("/") ? String(baseURL.dropLast()) : baseURL
        self.token = token
        self.skipTLSVerify = skipTLSVerify
    }

    // MARK: - Connection lifecycle

    func connect() {
        disconnect(reconnect: false)
        shouldReconnect = true

        let scheme = baseURL.hasPrefix("https") ? "wss" : "ws"
        let host = baseURL
            .replacingOccurrences(of: "https://", with: "")
            .replacingOccurrences(of: "http://", with: "")
        let urlStr = "\(scheme)://\(host)/ws/client?token=\(token.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? token)"
        guard let url = URL(string: urlStr) else { return }

        urlSession = URLSession(configuration: .default, delegate: self, delegateQueue: nil)
        var request = URLRequest(url: url)
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        task = urlSession?.webSocketTask(with: request)
        task?.resume()
        receiveLoop()
    }

    func disconnect(reconnect: Bool = false) {
        if !reconnect { shouldReconnect = false }
        task?.cancel(with: .goingAway, reason: nil)
        task = nil
        urlSession?.invalidateAndCancel()
        urlSession = nil
    }

    // MARK: - Send helpers

    func send(_ data: Data) {
        guard let text = String(data: data, encoding: .utf8) else { return }
        task?.send(.string(text)) { error in
            if let error { print("[ws] send error: \(error)") }
        }
    }

    func sendAttach(sessionID: String) {
        sendJSON(["type": "attach", "data": ["session_id": sessionID, "since_seq": 0]])
    }

    func sendTermIn(sessionID: String, dataB64: String) {
        sendJSON(["type": "term_in", "session_id": sessionID, "data_b64": dataB64])
    }

    func sendResize(sessionID: String, cols: Int, rows: Int) {
        sendJSON(["type": "resize", "session_id": sessionID, "data": ["cols": cols, "rows": rows]])
    }

    func sendAction(sessionID: String, kind: String) {
        sendJSON(["type": "action", "session_id": sessionID, "data": ["kind": kind]])
    }

    private func sendJSON(_ obj: [String: Any]) {
        guard let data = try? JSONSerialization.data(withJSONObject: obj) else { return }
        send(data)
    }

    // MARK: - Receive loop

    private func receiveLoop() {
        task?.receive { [weak self] result in
            guard let self else { return }
            switch result {
            case .success(let message):
                let text: String? = {
                    switch message {
                    case .string(let s): return s
                    case .data(let d): return String(data: d, encoding: .utf8)
                    @unknown default: return nil
                    }
                }()
                if let text, let msg = WSMessageParser.parse(text) {
                    DispatchQueue.main.async { self.onMessage?(msg) }
                }
                self.receiveLoop()
            case .failure(let error):
                print("[ws] receive error: \(error)")
                DispatchQueue.main.async { self.onConnectionChange?(false) }
                self.scheduleReconnect()
            }
        }
    }

    private func scheduleReconnect() {
        guard shouldReconnect else { return }
        let delay = reconnectDelay
        reconnectDelay = min(reconnectDelay * 2, 30.0) // exponential backoff, cap 30s
        print("[ws] reconnecting in \(delay)s")
        DispatchQueue.main.asyncAfter(deadline: .now() + delay) { [weak self] in
            guard let self, self.shouldReconnect else { return }
            self.connect()
        }
    }

    // MARK: - URLSessionWebSocketDelegate

    func urlSession(
        _ session: URLSession,
        webSocketTask: URLSessionWebSocketTask,
        didOpenWithProtocol proto: String?
    ) {
        print("[ws] connected")
        reconnectDelay = 1.0  // reset backoff on success
        DispatchQueue.main.async { self.onConnectionChange?(true) }
    }

    func urlSession(
        _ session: URLSession,
        webSocketTask: URLSessionWebSocketTask,
        didCloseWith closeCode: URLSessionWebSocketTask.CloseCode,
        reason: Data?
    ) {
        print("[ws] closed code=\(closeCode.rawValue)")
        DispatchQueue.main.async { self.onConnectionChange?(false) }
        scheduleReconnect()
    }

    // MARK: - URLSessionDelegate (TLS bypass for self-signed certs)

    func urlSession(
        _ session: URLSession,
        didReceive challenge: URLAuthenticationChallenge,
        completionHandler: @escaping (URLSession.AuthChallengeDisposition, URLCredential?) -> Void
    ) {
        if skipTLSVerify,
           challenge.protectionSpace.authenticationMethod == NSURLAuthenticationMethodServerTrust,
           let trust = challenge.protectionSpace.serverTrust {
            completionHandler(.useCredential, URLCredential(trust: trust))
            return
        }
        completionHandler(.performDefaultHandling, nil)
    }
}
