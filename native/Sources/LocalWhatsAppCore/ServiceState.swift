import Foundation

public struct WhatsAppState: Codable, Sendable {
    public var status: String?
    public var detail: String?
    public var qr: String?
    public var linked: Bool?
    public var lastEventAt: String?
    public init(status: String? = nil, detail: String? = nil, qr: String? = nil, lastEventAt: String? = nil, linked: Bool? = nil) {
        self.status = status; self.detail = detail; self.qr = qr; self.lastEventAt = lastEventAt; self.linked = linked
    }
    enum CodingKeys: String, CodingKey { case status, detail, qr, linked; case lastEventAt = "last_event_at" }
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
    public init(id: String, name: String, client: String? = nil, sendEnabled: Bool = false, connected: Bool = false) {
        self.id = id; self.name = name; self.client = client; self.sendEnabled = sendEnabled; self.connected = connected
    }
    enum CodingKeys: String, CodingKey {
        case id, name, client, connected
        case sendEnabled = "send_enabled", lastSeenAt = "last_seen_at"
    }
}
public struct MCPState: Codable, Sendable {
    public var url: String?
    public var status: String?
    public init(url: String? = nil, status: String? = nil) { self.url = url; self.status = status }
}
public struct CloudState: Codable, Sendable {
    public var enabled: Bool?
    public var provider: String?
    public var hostname: String?
    public var status: String?
    public var detail: String?
    public init(enabled: Bool = false, provider: String? = nil, hostname: String? = nil, status: String? = nil, detail: String? = nil) {
        self.enabled = enabled; self.provider = provider; self.hostname = hostname; self.status = status; self.detail = detail
    }
}
public struct ServiceState: Decodable, Sendable {
    public var whatsapp: WhatsAppState
    public var scope: Scope
    public var agents: [AgentConnection]
    public var mcp: MCPState
    public var cloud: CloudState
    public var error: String?
    public init(whatsapp: WhatsAppState = .init(), scope: Scope = .init(), agents: [AgentConnection] = [], mcp: MCPState = .init(), cloud: CloudState = .init()) {
        self.whatsapp = whatsapp; self.scope = scope; self.agents = agents; self.mcp = mcp; self.cloud = cloud
    }
    enum CodingKeys: String, CodingKey { case whatsapp, scope, agents, mcp, cloud, error }
    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        whatsapp = try c.decodeIfPresent(WhatsAppState.self, forKey: .whatsapp) ?? .init()
        scope = try c.decodeIfPresent(Scope.self, forKey: .scope) ?? .init()
        agents = try c.decodeIfPresent([AgentConnection].self, forKey: .agents) ?? []
        mcp = try c.decodeIfPresent(MCPState.self, forKey: .mcp) ?? .init()
        cloud = try c.decodeIfPresent(CloudState.self, forKey: .cloud) ?? .init()
        error = try c.decodeIfPresent(String.self, forKey: .error)
    }
}
public struct Catalog: Decodable { public var chats: [Chat] }
public struct SetupInstructions: Decodable {
    public var instructions: String
    public var expiresAt: String?
    enum CodingKeys: String, CodingKey { case instructions; case expiresAt = "expires_at" }
}
