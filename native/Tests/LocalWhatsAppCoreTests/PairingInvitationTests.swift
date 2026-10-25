import XCTest
@testable import LocalWhatsAppCore

final class PairingInvitationTests: XCTestCase {
    private func invitation(expiry: String? = "2026-10-23T12:00:00Z", instructions: String = "Synthetic setup instructions", invitationID: String = "invitation-a") -> PairingInvitation? {
        PairingInvitation(instructions: instructions, expiresAt: expiry, invitationID: invitationID, name: "Work agent", client: "codex", knownAgentIDs: ["existing"])
    }

    private func agent(id: String, invitationID: String?) -> AgentConnection {
        var agent = AgentConnection(id: id, name: "Work agent", client: "codex")
        agent.pairingInvitationID = invitationID
        return agent
    }

    func testExpiryIncludesExactBoundaryAndFractionalSeconds() throws {
        let plain = try XCTUnwrap(invitation())
        let fractional = try XCTUnwrap(invitation(expiry: "2026-10-23T12:00:00.125Z"))
        XCTAssertFalse(plain.isExpired(at: plain.expiresAt.addingTimeInterval(-0.001)))
        XCTAssertTrue(plain.isExpired(at: plain.expiresAt))
        XCTAssertTrue(plain.isExpired(at: plain.expiresAt.addingTimeInterval(1)))
        XCTAssertEqual(fractional.expiresAt.timeIntervalSince(plain.expiresAt), 0.125, accuracy: 0.001)
        XCTAssertEqual(invitation(expiry: "2026-10-23T05:00:00-07:00")?.expiresAt, plain.expiresAt)
    }

    func testMissingExpiryOrInstructionsCannotBecomePermanentInvitation() {
        XCTAssertNil(invitation(expiry: nil))
        XCTAssertNil(invitation(expiry: ""))
        XCTAssertNil(invitation(expiry: "not a date"))
        XCTAssertNil(invitation(instructions: ""))
        XCTAssertNil(invitation(instructions: " \n\t "))
        XCTAssertNil(invitation(invitationID: ""))
        XCTAssertNil(invitation(invitationID: " \n\t "))
    }

    func testOnlyNewAgentWithExactInvitationIDCompletesInvitation() throws {
        let invitation = try XCTUnwrap(invitation())
        let unrelated: [AgentConnection] = [
            agent(id: "existing", invitationID: "invitation-a"),
            agent(id: "other-invitation", invitationID: "invitation-b"),
            agent(id: "legacy-without-id", invitationID: nil)
        ]
        XCTAssertNil(invitation.matchingAgent(in: unrelated))
        var matching = agent(id: "new", invitationID: "invitation-a")
        matching.name = "Renamed agent"
        matching.client = nil
        XCTAssertEqual(invitation.matchingAgent(in: unrelated + [matching])?.id, "new")
    }

    func testSameNameAndClientInvitationsCannotCompleteEachOther() throws {
        let first = try XCTUnwrap(invitation(invitationID: "invitation-a"))
        let second = try XCTUnwrap(invitation(invitationID: "invitation-b"))
        XCTAssertEqual(first.name, second.name)
        XCTAssertEqual(first.client, second.client)
        let firstAgent = agent(id: "agent-a", invitationID: "invitation-a")
        let secondAgent = agent(id: "agent-b", invitationID: "invitation-b")
        XCTAssertNil(first.matchingAgent(in: [secondAgent]))
        XCTAssertNil(second.matchingAgent(in: [firstAgent]))
        XCTAssertEqual(first.matchingAgent(in: [secondAgent, firstAgent])?.id, "agent-a")
        XCTAssertEqual(second.matchingAgent(in: [firstAgent, secondAgent])?.id, "agent-b")
    }
}
