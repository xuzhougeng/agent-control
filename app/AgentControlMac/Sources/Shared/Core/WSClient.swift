import Foundation

final class WSClient: NSObject, URLSessionWebSocketDelegate, URLSessionDelegate {
    private static let protocolVersion = 1

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

    static func webSocketURLString(baseURL: String, token: String) -> String? {
        let normalizedBaseURL = baseURL.hasSuffix("/") ? String(baseURL.dropLast()) : baseURL
        let scheme = normalizedBaseURL.hasPrefix("https") ? "wss" : "ws"
        let host = normalizedBaseURL
            .replacingOccurrences(of: "https://", with: "")
            .replacingOccurrences(of: "http://", with: "")
        let encodedToken = token.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? token
        let urlStr = "\(scheme)://\(host)/ws/client?token=\(encodedToken)&v=\(protocolVersion)"
        guard URL(string: urlStr) != nil else { return nil }
        return urlStr
    }

    // MARK: - Connection lifecycle

    func connect() {
        if task != nil { return }
        disconnect(reconnect: false)
        shouldReconnect = true

        guard let urlStr = Self.webSocketURLString(baseURL: baseURL, token: token),
              let url = URL(string: urlStr) else { return }

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

    @discardableResult
    func send(_ data: Data) -> Bool {
        guard task != nil else { return false }
        guard let text = String(data: data, encoding: .utf8) else { return false }
        task?.send(.string(text)) { error in
            if let error { print("[ws] send error: \(error)") }
        }
        return true
    }

    @discardableResult
    func sendAttach(sessionID: String) -> Bool {
        sendJSON(["type": "attach", "data": ["session_id": sessionID, "since_seq": 0]])
    }

    @discardableResult
    func sendTermIn(sessionID: String, dataB64: String) -> Bool {
        sendJSON(["type": "term_in", "session_id": sessionID, "data_b64": dataB64])
    }

    @discardableResult
    func sendResize(sessionID: String, cols: Int, rows: Int) -> Bool {
        sendJSON(["type": "resize", "session_id": sessionID, "data": ["cols": cols, "rows": rows]])
    }

    @discardableResult
    func sendAction(sessionID: String, kind: String) -> Bool {
        sendJSON(["type": "action", "session_id": sessionID, "data": ["kind": kind]])
    }

    @discardableResult
    func sendChatIn(sessionID: String, content: String, contentParts: [ChatContentPart]) -> Bool {
        let payload: [String: Any] = [
            "content": content,
            "content_parts": contentParts.map { part in
                var out: [String: Any] = ["type": part.type]
                if let text = part.text { out["text"] = text }
                if let source = part.source {
                    out["source"] = [
                        "type": source.type,
                        "media_type": source.mediaType,
                        "data": source.data,
                    ]
                }
                return out
            },
        ]
        return sendJSON(["type": "chat_in", "session_id": sessionID, "data": payload])
    }

    @discardableResult
    private func sendJSON(_ obj: [String: Any]) -> Bool {
        guard let data = try? JSONSerialization.data(withJSONObject: obj) else { return false }
        return send(data)
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
        let reasonText = reason.flatMap { String(data: $0, encoding: .utf8) } ?? ""
        if reasonText.isEmpty {
            print("[ws] closed code=\(closeCode.rawValue)")
        } else {
            print("[ws] closed code=\(closeCode.rawValue), reason=\(reasonText)")
        }
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
