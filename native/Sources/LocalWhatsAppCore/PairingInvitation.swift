import Foundation

/// Temporary setup instructions remain separate from the resulting saved grant.
public struct PairingInvitation: Sendable {
    public let instructions: String
    public let expiresAt: Date
    public let invitationID: String
    public let name: String
    public let client: String
    private let knownAgentIDs: Set<String>

    public init?(instructions: String, expiresAt: String?, invitationID: String, name: String, client: String, knownAgentIDs: Set<String>) {
        guard !instructions.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              !invitationID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              let expiresAt else { return nil }
        let formatter = ISO8601DateFormatter()
        var expiry = formatter.date(from: expiresAt)
        if expiry == nil {
            formatter.formatOptions.insert(.withFractionalSeconds)
            expiry = formatter.date(from: expiresAt)
        }
        guard let expiry else { return nil }
        self.instructions = instructions
        self.expiresAt = expiry
        self.invitationID = invitationID
        self.name = name
        self.client = client
        self.knownAgentIDs = knownAgentIDs
    }

    public func isExpired(at now: Date) -> Bool { now >= expiresAt }

    public func matchingAgent(in agents: [AgentConnection]) -> AgentConnection? {
        agents.first { !knownAgentIDs.contains($0.id) && $0.pairingInvitationID == invitationID }
    }
}
