import Foundation

// MARK: - REST Models (mirrors cc-control/internal/core/model.go)

enum SessionType: String, Codable {
    case pty
    case chat
}

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

    /// True when the server is running the new in-process cc-agent runtime
    /// (vs the legacy cc-proxy PTY wrapper). Detected via the registered
    /// "cc-agent" tag or the synthetic claude_path="cc-agent-builtin".
    var isCCAgent: Bool {
        if let tags = tags, tags.contains("cc-agent") { return true }
        return claudePath == "cc-agent-builtin"
    }

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
    let sessionType: SessionType
    let cwd: String
    let cmd: [String]?
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
    var isChat: Bool { sessionType == .chat }
    var isPTY: Bool { sessionType == .pty }

    enum CodingKeys: String, CodingKey {
        case sessionID = "session_id"
        case serverID = "server_id"
        case sessionType = "session_type"
        case cwd, cmd
        case envKeys = "env_keys"
        case status
        case createdBy = "created_by"
        case createdAtMS = "created_at_ms"
        case exitCode = "exit_code"
        case exitReason = "exit_reason"
        case awaitingApproval = "awaiting_approval"
        case pendingEventID = "pending_event_id"
    }

    init(
        sessionID: String,
        serverID: String,
        sessionType: SessionType = .pty,
        cwd: String,
        cmd: [String]? = nil,
        envKeys: [String]? = nil,
        status: String,
        createdBy: String? = nil,
        createdAtMS: Int64? = nil,
        exitCode: Int? = nil,
        exitReason: String? = nil,
        awaitingApproval: Bool = false,
        pendingEventID: String? = nil
    ) {
        self.sessionID = sessionID
        self.serverID = serverID
        self.sessionType = sessionType
        self.cwd = cwd
        self.cmd = cmd
        self.envKeys = envKeys
        self.status = status
        self.createdBy = createdBy
        self.createdAtMS = createdAtMS
        self.exitCode = exitCode
        self.exitReason = exitReason
        self.awaitingApproval = awaitingApproval
        self.pendingEventID = pendingEventID
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        sessionID = try c.decode(String.self, forKey: .sessionID)
        serverID = try c.decode(String.self, forKey: .serverID)
        sessionType = try c.decodeIfPresent(SessionType.self, forKey: .sessionType) ?? .pty
        cwd = try c.decodeIfPresent(String.self, forKey: .cwd) ?? ""
        cmd = try c.decodeIfPresent([String].self, forKey: .cmd)
        envKeys = try c.decodeIfPresent([String].self, forKey: .envKeys)
        status = try c.decodeIfPresent(String.self, forKey: .status) ?? ""
        createdBy = try c.decodeIfPresent(String.self, forKey: .createdBy)
        createdAtMS = try c.decodeIfPresent(Int64.self, forKey: .createdAtMS)
        exitCode = try c.decodeIfPresent(Int.self, forKey: .exitCode)
        exitReason = try c.decodeIfPresent(String.self, forKey: .exitReason)
        awaitingApproval = try c.decodeIfPresent(Bool.self, forKey: .awaitingApproval) ?? false
        pendingEventID = try c.decodeIfPresent(String.self, forKey: .pendingEventID)
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

enum NotificationLevel: String, Codable {
    case info
    case success
    case warning
    case error
}

struct NotificationEvent: Identifiable {
    let notificationID: String
    let kind: String
    let tenantID: String?
    let sessionID: String?
    let serverID: String?
    let level: NotificationLevel
    let title: String?
    let message: String
    let source: String?
    let actor: String?
    let tsMS: Int64

    var id: String { notificationID }
    var displayTitle: String {
        let value = title?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return value.isEmpty ? "Notification" : value
    }
}

// MARK: - REST Response Wrappers

struct ServersResponse: Decodable { let servers: [Server] }
struct SessionsResponse: Decodable { let sessions: [Session] }

// MARK: - WS Message Types

enum WSMessage {
    case termOut(sessionID: String, data: Data, seq: UInt64)
    case event(SessionEvent)
    case notification(NotificationEvent)
    case sessionUpdate(SessionUpdatePayload)
    case chatMsg(ChatMessage)
    case attachOK(sessionID: String)
    case error(sessionID: String, message: String)
}

struct SessionUpdatePayload {
    let sessionID: String
    let status: String
    let exitCode: Int?
    let exitReason: String?
    let awaitingApproval: Bool
    let pendingEventID: String?
    let sessionType: SessionType?
}

struct ChatImageSource: Codable {
    let type: String
    let mediaType: String
    let data: String

    enum CodingKeys: String, CodingKey {
        case type
        case mediaType = "media_type"
        case data
    }
}

struct ChatContentPart: Codable, Identifiable {
    let type: String
    let text: String?
    let source: ChatImageSource?

    var id: String {
        if type == "text" {
            return "text:\(text ?? "")"
        }
        if type == "image" {
            return "img:\(source?.mediaType ?? ""):\(source?.data.prefix(16) ?? "")"
        }
        return "part:\(type):\(text ?? ""):\(source?.mediaType ?? "")"
    }
}

struct ChatMeta: Codable {
    let operations: [String]?
    let contentParts: [ChatContentPart]?
    let source: String?
    let progress: Bool?

    enum CodingKeys: String, CodingKey {
        case operations
        case contentParts = "content_parts"
        case source
        case progress
    }

    init(
        operations: [String]? = nil,
        contentParts: [ChatContentPart]? = nil,
        source: String? = nil,
        progress: Bool? = nil
    ) {
        self.operations = operations
        self.contentParts = contentParts
        self.source = source
        self.progress = progress
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        operations = try? c.decode([String].self, forKey: .operations)
        contentParts = try? c.decode([ChatContentPart].self, forKey: .contentParts)
        source = try? c.decode(String.self, forKey: .source)
        progress = try? c.decode(Bool.self, forKey: .progress)
    }
}

struct ChatMessage: Codable, Identifiable {
    let messageID: String
    let sessionID: String
    let role: String
    let content: String
    let meta: ChatMeta?
    let tsMS: Int64

    // `message_id` can be reused across roles (user + assistant final reply).
    // Use a composite key so SwiftUI list identity stays unique and stable.
    var id: String { "\(role):\(messageID):\(tsMS)" }
    var isUser: Bool { role == "user" }
    var isAssistant: Bool { role == "assistant" }

    enum CodingKeys: String, CodingKey {
        case messageID = "message_id"
        case sessionID = "session_id"
        case role
        case content
        case meta
        case tsMS = "ts_ms"
    }

    init(
        messageID: String,
        sessionID: String,
        role: String,
        content: String,
        meta: ChatMeta?,
        tsMS: Int64
    ) {
        self.messageID = messageID
        self.sessionID = sessionID
        self.role = role
        self.content = content
        self.meta = meta
        self.tsMS = tsMS
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        messageID = try c.decodeIfPresent(String.self, forKey: .messageID) ?? UUID().uuidString
        sessionID = try c.decodeIfPresent(String.self, forKey: .sessionID) ?? ""
        role = try c.decodeIfPresent(String.self, forKey: .role) ?? "assistant"
        content = try c.decodeIfPresent(String.self, forKey: .content) ?? ""
        meta = try c.decodeIfPresent(ChatMeta.self, forKey: .meta)
        tsMS = try c.decodeIfPresent(Int64.self, forKey: .tsMS) ?? Int64(Date().timeIntervalSince1970 * 1000)
    }
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
            guard let d = dataDict, let kind = d["kind"] as? String else { return nil }
            if kind == "approval_needed" {
                guard let ev = parseSessionEvent(d) else { return nil }
                return .event(ev)
            }
            if kind == "notification" {
                guard let ev = parseNotificationEvent(d) else { return nil }
                return .notification(ev)
            }
            return nil
        case "session_update":
            guard let d = dataDict else { return nil }
            return .sessionUpdate(parseSessionUpdate(d, fallbackID: sessionID))
        case "chat_msg":
            guard let d = dataDict,
                  let raw = try? JSONSerialization.data(withJSONObject: d),
                  let cm = try? JSONDecoder().decode(ChatMessage.self, from: raw) else { return nil }
            return .chatMsg(cm)
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

    private static func parseNotificationEvent(_ d: [String: Any]) -> NotificationEvent? {
        guard let notificationID = d["notification_id"] as? String,
              let message = d["message"] as? String,
              let tsMS = (d["ts_ms"] as? NSNumber)?.int64Value else { return nil }
        let levelRaw = (d["level"] as? String ?? "info").lowercased()
        let level = NotificationLevel(rawValue: levelRaw) ?? .info
        let sessionID = (d["session_id"] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines)
        let serverID = (d["server_id"] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines)
        return NotificationEvent(
            notificationID: notificationID,
            kind: d["kind"] as? String ?? "notification",
            tenantID: d["tenant_id"] as? String,
            sessionID: (sessionID?.isEmpty == false) ? sessionID : nil,
            serverID: (serverID?.isEmpty == false) ? serverID : nil,
            level: level,
            title: d["title"] as? String,
            message: message,
            source: d["source"] as? String,
            actor: d["actor"] as? String,
            tsMS: tsMS
        )
    }

    private static func parseSessionUpdate(_ d: [String: Any], fallbackID: String) -> SessionUpdatePayload {
        let sessionType: SessionType? = {
            guard let raw = d["session_type"] as? String else { return nil }
            return SessionType(rawValue: raw)
        }()
        return SessionUpdatePayload(
            sessionID: d["session_id"] as? String ?? fallbackID,
            status: d["status"] as? String ?? "",
            exitCode: (d["exit_code"] as? NSNumber)?.intValue,
            exitReason: d["exit_reason"] as? String,
            awaitingApproval: d["awaiting_approval"] as? Bool ?? false,
            pendingEventID: d["pending_event_id"] as? String,
            sessionType: sessionType
        )
    }
}
