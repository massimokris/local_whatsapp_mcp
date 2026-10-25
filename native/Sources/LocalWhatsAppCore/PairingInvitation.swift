import Foundation

/// Temporary setup instructions remain separate from the resulting saved grant.
public struct PairingInvitation: Sendable {
    public let instructions: String
    public let expiresAt: Date
    public let invitationID: String
    private let knownAgentIDs: Set<String>

    public init?(instructions: String, expiresAt: String?, invitationID: String, knownAgentIDs: Set<String>) {
        guard !instructions.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              !invitationID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              let expiry = parseServiceTimestamp(expiresAt) else { return nil }
        self.instructions = instructions
        self.expiresAt = expiry
        self.invitationID = invitationID
        self.knownAgentIDs = knownAgentIDs
    }

    public func isExpired(at now: Date) -> Bool { now >= expiresAt }

    public func matchingAgent(in agents: [AgentConnection]) -> AgentConnection? {
        agents.first { !knownAgentIDs.contains($0.id) && $0.pairingInvitationID == invitationID }
    }
}
