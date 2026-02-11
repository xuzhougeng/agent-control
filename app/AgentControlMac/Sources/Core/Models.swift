import Foundation

// MARK: - REST Models (mirrors cc-control/internal/core/model.go)

struct Server: Codable, Identifiable {
    let serverID: String
    let hostname: String
    let tags: [String]?
    let os: String?
    let arch: String?
    let agentVersion: String?
    let lastSeenMS: Int64?
    let status: String
    let allowRoots: [String]?
    let claudePath: String?

    var id: String { serverID }
    var isOnline: Bool { status == "online" }

    enum CodingKeys: String, CodingKey {
        case serverID = "server_id"
        case hostname, tags, os, arch
        case agentVersion = "agent_version"
        case lastSeenMS = "last_seen_ms"
        case status
        case allowRoots = "allow_roots"
        case claudePath = "claude_path"
    }
}

struct Session: Codable, Identifiable {
    let sessionID: String
    let serverID: String
    let cwd: String
    let cmd: [String]?
    let resumeID: String?
    let envKeys: [String]?
    let status: String
    let createdBy: String?
    let createdAtMS: Int64?
    let exitCode: Int?
    let exitReason: String?
    let awaitingApproval: Bool
    let pendingEventID: String?

    var id: String { sessionID }
    var isRunning: Bool { status == "running" }
    var shortID: String { String(sessionID.prefix(8)) }

    enum CodingKeys: String, CodingKey {
        case sessionID = "session_id"
        case serverID = "server_id"
        case cwd, cmd
        case resumeID = "resume_id"
        case envKeys = "env_keys"
        case status
        case createdBy = "created_by"
        case createdAtMS = "created_at_ms"
        case exitCode = "exit_code"
        case exitReason = "exit_reason"
        case awaitingApproval = "awaiting_approval"
        case pendingEventID = "pending_event_id"
    }
}

struct SessionEvent: Identifiable {
    let eventID: String
    let sessionID: String
    let serverID: String
    let kind: String
    let promptExcerpt: String?
    let actor: String?
    let tsMS: Int64
    var resolved: Bool

    var id: String { eventID }
}

// MARK: - REST Response Wrappers

struct ServersResponse: Decodable { let servers: [Server] }
struct SessionsResponse: Decodable { let sessions: [Session] }

// MARK: - WS Message Types

enum WSMessage {
    case termOut(sessionID: String, data: Data, seq: UInt64)
    case event(SessionEvent)
    case sessionUpdate(SessionUpdatePayload)
    case attachOK(sessionID: String)
    case error(sessionID: String, message: String)
}

struct SessionUpdatePayload {
    let sessionID: String
    let status: String
    let exitCode: Int?
    let exitReason: String?
    let resumeID: String?
    let awaitingApproval: Bool
    let pendingEventID: String?
}

// MARK: - WS Message Parser (JSONSerialization-based for flexible `data` field)

enum WSMessageParser {
    static func parse(_ text: String) -> WSMessage? {
        guard let raw = text.data(using: .utf8),
              let json = try? JSONSerialization.jsonObject(with: raw) as? [String: Any],
              let type = json["type"] as? String else { return nil }

        let sessionID = json["session_id"] as? String ?? ""
        let seq = (json["seq"] as? NSNumber)?.uint64Value ?? 0
        let dataB64 = json["data_b64"] as? String
        let dataDict = json["data"] as? [String: Any]

        switch type {
        case "term_out":
            guard let b64 = dataB64, let bytes = Data(base64Encoded: b64) else { return nil }
            return .termOut(sessionID: sessionID, data: bytes, seq: seq)
        case "event":
            guard let d = dataDict, let ev = parseSessionEvent(d) else { return nil }
            return .event(ev)
        case "session_update":
            guard let d = dataDict else { return nil }
            return .sessionUpdate(parseSessionUpdate(d, fallbackID: sessionID))
        case "attach_ok":
            return .attachOK(sessionID: sessionID)
        case "error":
            let msg = dataDict?["message"] as? String ?? "unknown error"
            return .error(sessionID: sessionID, message: msg)
        case "debug_probe":
            return nil
        default:
            return nil
        }
    }

    private static func parseSessionEvent(_ d: [String: Any]) -> SessionEvent? {
        guard let eventID = d["event_id"] as? String,
              let sessionID = d["session_id"] as? String,
              let serverID = d["server_id"] as? String,
              let kind = d["kind"] as? String,
              let tsMS = (d["ts_ms"] as? NSNumber)?.int64Value else { return nil }
        return SessionEvent(
            eventID: eventID, sessionID: sessionID, serverID: serverID,
            kind: kind, promptExcerpt: d["prompt_excerpt"] as? String,
            actor: d["actor"] as? String, tsMS: tsMS,
            resolved: d["resolved"] as? Bool ?? false
        )
    }

    private static func parseSessionUpdate(_ d: [String: Any], fallbackID: String) -> SessionUpdatePayload {
        SessionUpdatePayload(
            sessionID: d["session_id"] as? String ?? fallbackID,
            status: d["status"] as? String ?? "",
            exitCode: (d["exit_code"] as? NSNumber)?.intValue,
            exitReason: d["exit_reason"] as? String,
            resumeID: d["resume_id"] as? String,
            awaitingApproval: d["awaiting_approval"] as? Bool ?? false,
            pendingEventID: d["pending_event_id"] as? String
        )
    }
}
