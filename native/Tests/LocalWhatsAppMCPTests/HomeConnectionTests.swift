import AppKit
import XCTest
import LocalWhatsAppCore
@testable import LocalWhatsAppMCP

@MainActor
final class HomeConnectionTests: XCTestCase {
    func testHomeDisclosureClearsCredentialsOnCollapseNavigationAndPanelClose() throws {
        let model = makeDemoModel()
        let agent = try XCTUnwrap(model.homeAgent)
        model.toggleLocalMCP()
        XCTAssertEqual(model.page, .home)
        XCTAssertEqual(model.expandedAgentID, agent.id)
        XCTAssertEqual(model.connectionCredentials?.id, agent.id)

        model.toggleLocalMCP()
        XCTAssertEqual(model.page, .home)
        XCTAssertNil(model.expandedAgentID)
        XCTAssertNil(model.connectionCredentials)

        model.toggleLocalMCP()
        XCTAssertEqual(model.connectionCredentials?.id, agent.id)
        model.page = .settings
        XCTAssertNil(model.connectionCredentials)
        XCTAssertNil(model.expandedAgentID)

        model.page = .home
        model.toggleLocalMCP()
        XCTAssertEqual(model.connectionCredentials?.id, agent.id)
        model.panelClosed()
        XCTAssertNil(model.connectionCredentials)
        XCTAssertNil(model.expandedAgentID)
    }

    func testHomeActionsOnlyChangeTheDisplayedConnection() throws {
        let model = makeDemoModel()
        let agent = try XCTUnwrap(model.homeAgent)
        let other = try XCTUnwrap(model.agents.last)
        model.toggleLocalMCP()
        model.changeSending(agent, enabled: true)
        XCTAssertEqual(model.state.agents.first(where: { $0.id == agent.id })?.sendEnabled, true)
        XCTAssertEqual(model.state.agents.first(where: { $0.id == other.id })?.sendEnabled, other.sendEnabled)
        model.changeSending(agent, enabled: false)
        XCTAssertEqual(model.state.agents.first(where: { $0.id == agent.id })?.sendEnabled, false)
        XCTAssertEqual(model.state.agents.first(where: { $0.id == other.id })?.sendEnabled, other.sendEnabled)
        model.removeAgent(agent)
        XCTAssertEqual(model.state.agents.map(\.id), [other.id])
        XCTAssertEqual(model.page, .home)
        XCTAssertNil(model.expandedAgentID)
    }

    func testEmptyHomeStartsInitialPairing() {
        let model = makeDemoModel()
        model.state.agents = []
        model.toggleLocalMCP()
        XCTAssertEqual(model.page, .pairing)
        XCTAssertNil(model.expandedAgentID)
        XCTAssertTrue(model.canCreateConnection)
    }

    func testHomeLoadsExistingCredentialsThroughRuntime() async throws {
        let (model, fixture) = try await makeLiveModel()
        let agent = try XCTUnwrap(model.homeAgent)
        model.toggleLocalMCP()
        try await waitUntil { fixture.credentialsRequested }
        XCTAssertTrue(model.loadingCredentials)
        fixture.releaseCredentials()
        try await waitUntil { !model.busy }
        XCTAssertEqual(model.page, .home)
        XCTAssertEqual(model.expandedAgentID, agent.id)
        XCTAssertEqual(model.connectionCredentials?.id, agent.id)
        XCTAssertEqual(model.connectionCredentials?.accessToken, "SYNTHETIC-HOME-CREDENTIAL")
        XCTAssertEqual(model.agents.count, 2)
        XCTAssertEqual(try fixture.requests().filter { $0 == "GET /agents/first/credentials" }.count, 1)

        model.toggleLocalMCP()
        model.toggleLocalMCP()
        try await waitUntil { !model.busy }
        XCTAssertEqual(model.connectionCredentials?.accessToken, "SYNTHETIC-HOME-CREDENTIAL")
        XCTAssertEqual(model.agents.count, 2)
        XCTAssertEqual(try fixture.requests().filter { $0 == "GET /agents/first/credentials" }.count, 2)
        XCTAssertFalse(try fixture.requests().contains { $0.hasPrefix("POST ") })
    }

    func testCollapsingHomeDiscardsDelayedCredentials() async throws {
        let (model, fixture) = try await makeLiveModel()
        model.toggleLocalMCP()
        try await waitUntil { fixture.credentialsRequested }
        model.toggleLocalMCP()
        fixture.releaseCredentials()
        try await waitUntil { !model.busy }
        XCTAssertEqual(model.page, .home)
        XCTAssertNil(model.expandedAgentID)
        XCTAssertNil(model.connectionCredentials)
        XCTAssertNil(model.credentialsConnectionID)
    }

    func testReopeningDuringAutoloadLoadsOnlyTheCurrentDisclosure() async throws {
        let (model, fixture) = try await makeLiveModel()
        model.toggleLocalMCP()
        try await waitUntil { fixture.credentialsRequested }
        model.toggleLocalMCP()
        model.toggleLocalMCP()
        XCTAssertNil(model.connectionCredentials)
        fixture.releaseCredentials()
        try await waitUntil { !model.busy }
        XCTAssertEqual(model.connectionCredentials?.id, model.homeAgent?.id)
        XCTAssertEqual(try fixture.requests().filter { $0 == "GET /agents/first/credentials" }.count, 2)
    }

    func testFailedAutoloadDoesNotRepeatOnPollingUntilReopened() async throws {
        let (model, fixture) = try await makeLiveModel()
        try fixture.failCredentials()
        model.toggleLocalMCP()
        try await waitUntil { fixture.credentialsRequested }
        fixture.releaseCredentials()
        try await waitUntil { !model.busy }
        XCTAssertNil(model.connectionCredentials)
        XCTAssertEqual(model.credentialsConnectionID, model.homeAgent?.id)
        XCTAssertNotNil(model.error)
        await model.refresh()
        model.reconcileConnections()
        await model.refresh()
        XCTAssertEqual(try fixture.requests().filter { $0 == "GET /agents/first/credentials" }.count, 1)

        model.toggleLocalMCP()
        model.toggleLocalMCP()
        try await waitUntil { !model.busy }
        XCTAssertEqual(try fixture.requests().filter { $0 == "GET /agents/first/credentials" }.count, 2)
    }

    func testExpandedHomeLoadsCredentialsWhenServiceReturns() async throws {
        let (model, fixture) = try await makeLiveModel()
        try fixture.setStateAvailable(false)
        await model.refresh()
        XCTAssertFalse(model.serviceAvailable)
        model.toggleLocalMCP()
        XCTAssertNotNil(model.expandedAgentID)
        XCTAssertFalse(fixture.credentialsRequested)

        try fixture.setStateAvailable(true)
        await model.refresh()
        try await waitUntil { fixture.credentialsRequested }
        fixture.releaseCredentials()
        try await waitUntil { !model.busy }
        XCTAssertEqual(model.connectionCredentials?.id, model.homeAgent?.id)
        XCTAssertEqual(try fixture.requests().filter { $0 == "GET /agents/first/credentials" }.count, 1)
    }

    func testChangingDisplayedConnectionDiscardsDelayedCredentials() async throws {
        let (model, fixture) = try await makeLiveModel()
        let agent = try XCTUnwrap(model.homeAgent)
        model.toggleLocalMCP()
        try await waitUntil { fixture.credentialsRequested }
        let otherIndex = try XCTUnwrap(model.state.agents.firstIndex(where: { $0.id != agent.id }))
        model.state.agents[otherIndex].name = "Assistant"
        model.reconcileConnections()
        fixture.releaseCredentials()
        try await waitUntil { !model.busy }
        XCTAssertNotEqual(model.homeAgent?.id, agent.id)
        XCTAssertEqual(model.agents.count, 2)
        XCTAssertNil(model.connectionCredentials)
        XCTAssertNil(model.credentialsConnectionID)
        XCTAssertNil(model.expandedAgentID)
    }

    private func makeDemoModel() -> AppModel {
        let pasteboard = NSPasteboard(name: .init("LocalWhatsAppMCP-HomeTests-\(UUID().uuidString)"))
        addTeardownBlock { @MainActor in pasteboard.releaseGlobally() }
        let model = AppModel(environment: ["LWMCP_DEMO": "1", "LWMCP_DEMO_PAGE": "mcp-agents"], pasteboard: pasteboard)
        model.page = .home
        return model
    }

    private func makeLiveModel() async throws -> (AppModel, HomeCredentialsFixture) {
        let fixture = try HomeCredentialsFixture()
        let pasteboard = NSPasteboard(name: .init("LocalWhatsAppMCP-HomeTests-\(UUID().uuidString)"))
        let model = AppModel(environment: [:], pasteboard: pasteboard, runtime: fixture.runtime)
        addTeardownBlock { @MainActor in
            fixture.releaseCredentials()
            await model.stop()
            pasteboard.releaseGlobally()
            try? FileManager.default.removeItem(at: fixture.directory)
        }
        await model.refresh()
        XCTAssertEqual(model.page, .home)
        XCTAssertTrue(model.serviceAvailable)
        return (model, fixture)
    }

    private func waitUntil(file: StaticString = #filePath, line: UInt = #line, _ condition: () -> Bool) async throws {
        for _ in 0..<500 {
            if condition() { return }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTFail("Timed out waiting for synthetic credentials", file: file, line: line)
        throw HomeFixtureFailure.timedOut
    }
}

private enum HomeFixtureFailure: Error { case timedOut }

/// Uses the production helper request path with a delayed, synthetic credential response.
private struct HomeCredentialsFixture {
    let directory: URL
    let runtime: HelperRuntime

    init() throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent("LocalWhatsAppMCP-HomeCredentials-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let helper = directory.appendingPathComponent("helper")
        runtime = HelperRuntime(helper: helper, wacli: helper, dataDirectory: directory)
        let state: [String: Any] = [
            "whatsapp": ["linked": true, "status": "ready", "sync_state": "caught_up"],
            "scope": ["mode": "selected", "chat_ids": ["team"]],
            "agents": [["id": "first", "name": "Connection 1", "send_enabled": false], ["id": "other", "name": "Connection 2", "send_enabled": true]],
            "mcp": ["url": "http://127.0.0.1:32191/mcp", "status": "ready"]
        ]
        try JSONSerialization.data(withJSONObject: state).write(to: directory.appendingPathComponent("state.json"))
        try JSONSerialization.data(withJSONObject: ["id": "first", "name": "Connection 1", "url": "http://127.0.0.1:32191/mcp", "access_token": "SYNTHETIC-HOME-CREDENTIAL"]).write(to: directory.appendingPathComponent("credentials.json"))
        try Data().write(to: directory.appendingPathComponent("credentials-hold"))
        try Data("""
        #!/bin/sh
        if [ "$1" = "serve" ]; then exec /bin/cat >/dev/null; fi
        shift
        while [ "$#" -gt 0 ]; do
          case "$1" in
            --data-dir) fixture_dir="$2"; shift 2 ;;
            --method) method="$2"; shift 2 ;;
            --path) request_path="$2"; shift 2 ;;
            *) exit 1 ;;
          esac
        done
        /usr/bin/printf '%s %s\\n' "$method" "$request_path" >> "$fixture_dir/requests.log"
        case "$method $request_path" in
          'GET /state')
            if [ -e "$fixture_dir/state-unavailable" ]; then exit 1; fi
            /bin/cat "$fixture_dir/state.json"
            ;;
          'GET /catalog') /usr/bin/printf '%s' '{"chats":[]}' ;;
          'GET /agents/first/credentials')
            /usr/bin/touch "$fixture_dir/credentials-requested"
            while [ -e "$fixture_dir/credentials-hold" ]; do /bin/sleep 0.01; done
            /bin/cat "$fixture_dir/credentials.json"
            ;;
          *) exit 1 ;;
        esac
        """.utf8).write(to: helper)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: helper.path)
    }

    var credentialsRequested: Bool { FileManager.default.fileExists(atPath: directory.appendingPathComponent("credentials-requested").path) }
    func releaseCredentials() { try? FileManager.default.removeItem(at: directory.appendingPathComponent("credentials-hold")) }
    func requests() throws -> [String] {
        try String(contentsOf: directory.appendingPathComponent("requests.log"), encoding: .utf8).split(separator: "\n").map(String.init)
    }
    func failCredentials() throws {
        try JSONSerialization.data(withJSONObject: ["error": "Synthetic credential read failure"]).write(to: directory.appendingPathComponent("credentials.json"))
    }
    func setStateAvailable(_ available: Bool) throws {
        let flag = directory.appendingPathComponent("state-unavailable")
        if available { try FileManager.default.removeItem(at: flag) }
        else { try Data().write(to: flag) }
    }
}
