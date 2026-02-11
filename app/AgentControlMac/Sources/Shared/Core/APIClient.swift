import Foundation

enum APIError: LocalizedError {
    case invalidURL
    case httpError(Int, String)

    var errorDescription: String? {
        switch self {
        case .invalidURL: return "Invalid URL"
        case .httpError(let code, let body): return "HTTP \(code): \(body)"
        }
    }
}

final class APIClient {
    private(set) var baseURL = ""
    private(set) var token = ""
    private var skipTLSVerify = false
    private var customSession: URLSession?
    private var tlsBypassDelegate: TLSBypassDelegate?

    private var session: URLSession {
        if skipTLSVerify {
            if customSession == nil {
                tlsBypassDelegate = TLSBypassDelegate()
                customSession = URLSession(configuration: .default, delegate: tlsBypassDelegate, delegateQueue: nil)
            }
            return customSession!
        }
        customSession?.invalidateAndCancel()
        customSession = nil
        tlsBypassDelegate = nil
        return URLSession.shared
    }

    func configure(baseURL: String, token: String, skipTLSVerify: Bool = false) {
        self.baseURL = baseURL.hasSuffix("/") ? String(baseURL.dropLast()) : baseURL
        self.token = token
        if !skipTLSVerify {
            customSession?.invalidateAndCancel()
            customSession = nil
            tlsBypassDelegate = nil
        }
        self.skipTLSVerify = skipTLSVerify
    }

    /// One-off connectivity check with given config (does not change instance state).
    static func checkConnection(baseURL: String, token: String, skipTLSVerify: Bool) async throws {
        let base = baseURL.hasSuffix("/") ? String(baseURL.dropLast()) : baseURL
        guard let url = URL(string: base + "/api/servers") else { throw APIError.invalidURL }
        var req = URLRequest(url: url)
        req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        let session: URLSession
        if skipTLSVerify {
            let delegate = TLSBypassDelegate()
            session = URLSession(configuration: .default, delegate: delegate, delegateQueue: nil)
        } else {
            session = URLSession.shared
        }
        let (data, response) = try await session.data(for: req)
        if skipTLSVerify { session.invalidateAndCancel() }
        guard let http = response as? HTTPURLResponse else { throw APIError.invalidURL }
        if !(200...299).contains(http.statusCode) {
            throw APIError.httpError(http.statusCode, String(data: data, encoding: .utf8) ?? "")
        }
    }

    // MARK: - REST endpoints

    func fetchServers() async throws -> [Server] {
        let data = try await request("/api/servers")
        return try JSONDecoder().decode(ServersResponse.self, from: data).servers
    }

    func fetchSessions(serverID: String?) async throws -> [Session] {
        var path = "/api/sessions"
        if let sid = serverID, !sid.isEmpty {
            path += "?server_id=\(sid)"
        }
        let data = try await request(path)
        return try JSONDecoder().decode(SessionsResponse.self, from: data).sessions
    }

    func createSession(
        serverID: String, cwd: String, resumeID: String?,
        env: [String: String], cols: Int, rows: Int
    ) async throws -> Session {
        var body: [String: Any] = [
            "server_id": serverID, "cwd": cwd,
            "env": env, "cols": cols, "rows": rows,
        ]
        if let rid = resumeID, !rid.isEmpty { body["resume_id"] = rid }
        let jsonData = try JSONSerialization.data(withJSONObject: body)
        let data = try await request("/api/sessions", method: "POST", body: jsonData)
        return try JSONDecoder().decode(Session.self, from: data)
    }

    func stopSession(_ sessionID: String, graceMS: Int = 4000, killAfterMS: Int = 9000) async throws {
        let body = try JSONSerialization.data(withJSONObject: [
            "grace_ms": graceMS, "kill_after_ms": killAfterMS,
        ])
        _ = try await request("/api/sessions/\(sessionID)/stop", method: "POST", body: body)
    }

    func fetchEvents(_ sessionID: String) async throws -> [[String: Any]] {
        let data = try await request("/api/sessions/\(sessionID)/events")
        guard let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let events = json["events"] as? [[String: Any]] else { return [] }
        return events
    }

    // MARK: - Internal

    private func request(_ path: String, method: String = "GET", body: Data? = nil) async throws -> Data {
        guard let url = URL(string: baseURL + path) else { throw APIError.invalidURL }
        var req = URLRequest(url: url)
        req.httpMethod = method
        req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        if let body {
            req.httpBody = body
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        let (data, response) = try await session.data(for: req)
        if let http = response as? HTTPURLResponse, !(200...299).contains(http.statusCode) {
            throw APIError.httpError(http.statusCode, String(data: data, encoding: .utf8) ?? "")
        }
        return data
    }
}
