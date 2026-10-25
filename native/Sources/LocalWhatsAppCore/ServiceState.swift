import Foundation

public struct WhatsAppState: Codable, Sendable {
    public var status: String?
    public var detail: String?
    public var qr: String?
    public var linked: Bool?
    public var disconnectPending: Bool?
    public var lastEventAt: String?
    public var lastSyncedAt: String?
    public init(status: String? = nil, detail: String? = nil, qr: String? = nil, lastEventAt: String? = nil, linked: Bool? = nil, disconnectPending: Bool? = nil, lastSyncedAt: String? = nil) {
        self.status = status; self.detail = detail; self.qr = qr; self.lastEventAt = lastEventAt; self.linked = linked; self.disconnectPending = disconnectPending
        self.lastSyncedAt = lastSyncedAt
    }
    enum CodingKeys: String, CodingKey { case status, detail, qr, linked; case lastEventAt = "last_event_at"; case lastSyncedAt = "last_synced_at"; case disconnectPending = "disconnect_pending" }
}
public struct Scope: Codable, Sendable {
    public var mode: String
    public var chatIDs: [String]
    public var revision: Int
    public var isConfirmed: Bool { mode == "all" || mode == "selected" }
    public init(mode: String = "initial", chatIDs: [String] = [], revision: Int = 0) {
        self.mode = mode; self.chatIDs = chatIDs; self.revision = revision
    }
    enum CodingKeys: String, CodingKey { case mode, revision; case chatIDs = "chat_ids" }
    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        mode = try c.decodeIfPresent(String.self, forKey: .mode) ?? "initial"
        chatIDs = try c.decodeIfPresent([String].self, forKey: .chatIDs) ?? []
        revision = try c.decodeIfPresent(Int.self, forKey: .revision) ?? 0
    }
}
public struct AgentConnection: Codable, Identifiable, Sendable {
    public var id: String
    public var name: String
    public var client: String?
    public var sendEnabled: Bool?
    public var connected: Bool?
    public var lastSeenAt: String?
    public var pairingInvitationID: String?
    public init(id: String, name: String, client: String? = nil, sendEnabled: Bool = false, connected: Bool = false, pairingInvitationID: String? = nil) {
        self.id = id; self.name = name; self.client = client; self.sendEnabled = sendEnabled; self.connected = connected
        self.pairingInvitationID = pairingInvitationID
    }
    enum CodingKeys: String, CodingKey {
        case id, name, client, connected
        case sendEnabled = "send_enabled", lastSeenAt = "last_seen_at", pairingInvitationID = "pairing_invitation_id"
    }
}
public struct MCPState: Codable, Sendable {
    public var url: String?
    public var status: String?
    public init(url: String? = nil, status: String? = nil) { self.url = url; self.status = status }
}
public struct ServiceState: Decodable, Sendable {
    public var whatsapp: WhatsAppState
    public var scope: Scope
    public var agents: [AgentConnection]
    public var mcp: MCPState
    public var error: String?
    public init(whatsapp: WhatsAppState = .init(), scope: Scope = .init(), agents: [AgentConnection] = [], mcp: MCPState = .init()) {
        self.whatsapp = whatsapp; self.scope = scope; self.agents = agents; self.mcp = mcp
    }
    enum CodingKeys: String, CodingKey { case whatsapp, scope, agents, mcp, error }
    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        whatsapp = try c.decodeIfPresent(WhatsAppState.self, forKey: .whatsapp) ?? .init()
        scope = try c.decodeIfPresent(Scope.self, forKey: .scope) ?? .init()
        agents = try c.decodeIfPresent([AgentConnection].self, forKey: .agents) ?? []
        mcp = try c.decodeIfPresent(MCPState.self, forKey: .mcp) ?? .init()
        error = try c.decodeIfPresent(String.self, forKey: .error)
    }
}
public struct Catalog: Decodable { public var chats: [Chat] }
public struct SetupInstructions: Decodable {
    public var instructions: String
    public var expiresAt: String?
    public var invitationID: String?
    enum CodingKeys: String, CodingKey { case instructions; case expiresAt = "expires_at", invitationID = "invitation_id" }
}
