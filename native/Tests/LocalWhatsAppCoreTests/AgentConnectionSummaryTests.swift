import XCTest
@testable import LocalWhatsAppCore

final class AgentConnectionSummaryTests: XCTestCase {
    func testSavedGrantWithoutRequestDoesNotInventActivity() {
        let grant = AgentConnection(id: "a", name: "Agent 1", connected: true)
        XCTAssertEqual(AgentConnectionSummary(agent: grant).text(), "No request yet - Message sending off")
    }

    func testLastRequestAgeAndSendingAreIndependent() throws {
        let now = try XCTUnwrap(ISO8601DateFormatter().date(from: "2026-10-25T02:00:00Z"))
        var grant = AgentConnection(id: "a", name: "Agent 1", sendEnabled: true)
        grant.lastSeenAt = "2026-10-25T01:58:00.000Z"
        XCTAssertEqual(AgentConnectionSummary(agent: grant).text(at: now), "Last request 2m ago - Message sending on")
        grant.sendEnabled = false
        grant.lastSeenAt = "2026-10-24T00:00:00Z"
        XCTAssertEqual(AgentConnectionSummary(agent: grant).text(at: now), "Last request 1d ago - Message sending off")
        grant.lastSeenAt = "invalid"
        XCTAssertEqual(AgentConnectionSummary(agent: grant).text(at: now), "No request yet - Message sending off")
    }

    func testAutomaticNameAvoidsExistingNamesWithoutRenamingThem() {
        let agents = [AgentConnection(id: "a", name: "CONNECTION 1"), AgentConnection(id: "b", name: "Connection 2"), AgentConnection(id: "c", name: "Agent 3")]
        XCTAssertEqual(AgentConnectionSummary.nextName(in: agents), "Connection 3")
        XCTAssertEqual(AgentConnectionSummary.nextName(in: []), "Connection 1")
        XCTAssertEqual(AgentConnectionSummary.nextName(in: [agents[2]]), "Connection 1")
        XCTAssertEqual(agents.map(\.name), ["CONNECTION 1", "Connection 2", "Agent 3"])
    }
}
