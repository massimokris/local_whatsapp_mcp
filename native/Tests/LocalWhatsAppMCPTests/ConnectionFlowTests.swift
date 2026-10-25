import AppKit
import XCTest
import LocalWhatsAppCore
@testable import LocalWhatsAppMCP

@MainActor
final class ConnectionFlowTests: XCTestCase {
    private func model(page: String = "mcp") -> AppModel {
        AppModel(environment: ["LWMCP_DEMO": "1", "LWMCP_DEMO_PAGE": page],
                 pasteboard: NSPasteboard(name: .init("LocalWhatsAppMCPTests-\(UUID().uuidString)")))
    }

    func testManualDetailsReuseConnectionUntilSetupEnds() async throws {
        let model = model()
        model.toggleManualSetup()
        let original = try XCTUnwrap(model.connectionCredentials)
        XCTAssertEqual(model.page, .pairing)
        XCTAssertEqual(model.agents.count, 1)
        XCTAssertEqual(model.agents.first?.name, "Connection 1")
        XCTAssertNotEqual(model.agents.first?.sendEnabled, true)
        model.toggleManualSetup()
        XCTAssertNil(model.connectionCredentials)
        model.toggleManualSetup()
        XCTAssertEqual(model.connectionCredentials?.id, original.id)
        XCTAssertEqual(model.connectionCredentials?.accessToken, original.accessToken)
        XCTAssertEqual(model.agents.count, 1)
        model.finishConnectionSetup()
        XCTAssertEqual(model.page, .mcp)
        XCTAssertNil(model.connectionCredentials)
        model.beginPairing()
        model.toggleManualSetup()
        XCTAssertEqual(model.agents.count, 2)
        XCTAssertEqual(model.connectionCredentials?.name, "Connection 2")
    }

    func testPanelCloseClearsDetailsAndReopenReusesManualConnection() async throws {
        let model = model()
        model.toggleManualSetup()
        let originalID = try XCTUnwrap(model.connectionCredentials?.id)
        model.panelClosed()
        XCTAssertFalse(model.showingManualSetup)
        XCTAssertNil(model.connectionCredentials)
        model.toggleManualSetup()
        XCTAssertEqual(model.connectionCredentials?.id, originalID)
        XCTAssertEqual(model.agents.count, 1)
    }

    func testHiddenManualConnectionDeletionDoesNotLeaveStaleSetup() async throws {
        let model = model()
        model.toggleManualSetup()
        let originalID = try XCTUnwrap(model.connectionCredentials?.id)
        model.toggleManualSetup()
        model.state.agents = []
        model.reconcileConnections()
        model.toggleManualSetup()
        XCTAssertNotEqual(model.connectionCredentials?.id, originalID)
        XCTAssertEqual(model.agents.count, 1)
        XCTAssertTrue(model.showingManualSetup)
    }

    func testExistingCredentialsCollapseNavigateAndDeleteWithoutNewGrants() async throws {
        let model = model(page: "mcp-agents")
        let agent = try XCTUnwrap(model.agents.first)
        model.toggleAgent(agent)
        model.toggleCredentials(agent)
        XCTAssertEqual(model.connectionCredentials?.id, agent.id)
        XCTAssertEqual(model.agents.count, 2)
        model.toggleAgent(agent)
        XCTAssertNil(model.connectionCredentials)
        XCTAssertNil(model.expandedAgentID)
        model.toggleAgent(agent)
        model.toggleCredentials(agent)
        model.page = .home
        XCTAssertNil(model.connectionCredentials)
        XCTAssertNil(model.expandedAgentID)
        model.page = .mcp
        for saved in model.agents { model.removeAgent(saved) }
        XCTAssertTrue(model.agents.isEmpty)
        XCTAssertEqual(model.page, .mcp)
        XCTAssertTrue(model.canCreateConnection)
    }

    func testSendingOnlyChangesChosenConnection() async throws {
        let model = model(page: "mcp-agents")
        let agent = try XCTUnwrap(model.agents.first)
        let other = try XCTUnwrap(model.agents.last)
        model.changeSending(agent, enabled: true)
        XCTAssertEqual(model.state.agents.first(where: { $0.id == agent.id })?.sendEnabled, true)
        XCTAssertEqual(model.state.agents.first(where: { $0.id == other.id })?.sendEnabled, other.sendEnabled)
        model.changeSending(agent, enabled: false)
        XCTAssertEqual(model.state.agents.first(where: { $0.id == agent.id })?.sendEnabled, false)
    }

    func testCopyReusesInvitationAndSurvivesDone() async throws {
        let pasteboard = NSPasteboard(name: .init("LocalWhatsAppMCPTests-\(UUID().uuidString)"))
        defer { pasteboard.releaseGlobally() }
        let model = AppModel(environment: ["LWMCP_DEMO": "1", "LWMCP_DEMO_PAGE": "mcp"], pasteboard: pasteboard)
        model.beginPairing()
        model.copyInstructions()
        let invitation = try XCTUnwrap(model.invitation)
        model.copyInstructions()
        XCTAssertEqual(model.invitation?.invitationID, invitation.invitationID)
        XCTAssertEqual(pasteboard.string(forType: .string), invitation.instructions)
        model.finishConnectionSetup()
        XCTAssertNil(model.invitation)
        XCTAssertEqual(pasteboard.string(forType: .string), invitation.instructions)
    }

    func testOnlyMatchingInvitationReturnsToConnectionList() async throws {
        let model = model()
        defer { model.clearInvitation() }
        model.beginPairing()
        model.copyInstructions()
        let invitation = try XCTUnwrap(model.invitation)
        model.state.agents = [.init(id: "unrelated", name: "Connection 1", client: "generic", pairingInvitationID: "different")]
        model.reconcileConnections()
        XCTAssertEqual(model.page, .pairing)
        XCTAssertNotNil(model.invitation)
        model.state.agents.append(.init(id: "paired", name: "Connection 1", client: "generic", pairingInvitationID: invitation.invitationID))
        model.reconcileConnections()
        XCTAssertEqual(model.page, .mcp)
        XCTAssertNil(model.invitation)
    }

    func testCredentialCopyUsesFullValuesAndSurvivesCollapse() async throws {
        let pasteboard = NSPasteboard(name: .init("LocalWhatsAppMCPTests-\(UUID().uuidString)"))
        defer { pasteboard.releaseGlobally() }
        let model = AppModel(environment: ["LWMCP_DEMO": "1", "LWMCP_DEMO_PAGE": "mcp-agents"], pasteboard: pasteboard)
        let agent = try XCTUnwrap(model.agents.first)
        model.toggleAgent(agent)
        model.toggleCredentials(agent)
        let credentials = try XCTUnwrap(model.connectionCredentials)
        model.copyCredentialsURL()
        XCTAssertEqual(pasteboard.string(forType: .string), credentials.url)
        model.copyAccessToken()
        XCTAssertEqual(pasteboard.string(forType: .string), credentials.accessToken)
        model.toggleAgent(agent)
        XCTAssertNil(model.connectionCredentials)
        XCTAssertEqual(pasteboard.string(forType: .string), credentials.accessToken)
    }
}
