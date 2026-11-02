import AppKit
import XCTest
import LocalWhatsAppCore
@testable import LocalWhatsAppMCP

/// Records each confirmation request. Shared by tests that run AppModel against a live fixture,
/// so the suite never opens a real system prompt.
final class FakePresence: UserPresenceVerifying, @unchecked Sendable {
    var result: Bool
    private(set) var reasons: [String] = []
    init(result: Bool) { self.result = result }
    func verify(reason: String) async -> Bool { reasons.append(reason); return result }
}

private final class TestClock { var current = Date(timeIntervalSince1970: 1_000_000) }

@MainActor
final class PresenceGateTests: XCTestCase {
    private static let first: [String: Any] = ["id": "first", "name": "Connection 1", "send_enabled": false]
    private static let other: [String: Any] = ["id": "other", "name": "Connection 2", "send_enabled": true]
    private static let canceled = "Confirmation canceled. Nothing changed."

    func testEnablingSendingDeniedSendsNothing() async throws {
        let presence = FakePresence(result: false)
        let (model, fixture) = try await makeModel(presence: presence)
        model.changeSending(try agent("first", in: model), enabled: true)
        try await waitUntil { model.error != nil }
        XCTAssertEqual(model.error, Self.canceled)
        XCTAssertEqual(presence.reasons, ["allow Connection 1 to send WhatsApp messages"])
        XCTAssertFalse(try fixture.requests().contains { $0.hasPrefix("PATCH ") })
        XCTAssertEqual(try agent("first", in: model).sendEnabled, false)
    }

    func testEnablingSendingAllowedPatches() async throws {
        let presence = FakePresence(result: true)
        let (model, fixture) = try await makeModel(presence: presence)
        model.changeSending(try agent("first", in: model), enabled: true)
        try await waitUntil { (try? fixture.requests().contains("PATCH /agents/first")) == true && !model.busy }
        XCTAssertEqual(try fixture.requests().filter { $0.hasPrefix("PATCH ") }, ["PATCH /agents/first"])
        XCTAssertEqual(presence.reasons.count, 1)
    }

    func testDisablingSendingNeverPrompts() async throws {
        let presence = FakePresence(result: false)
        let (model, fixture) = try await makeModel(presence: presence)
        model.changeSending(try agent("other", in: model), enabled: false)
        try await waitUntil { (try? fixture.requests().contains("PATCH /agents/other")) == true && !model.busy }
        XCTAssertTrue(presence.reasons.isEmpty)
        XCTAssertNil(model.error)
    }

    func testInitialScopeConfirmationNeverPrompts() async throws {
        let presence = FakePresence(result: false)
        let (model, fixture) = try await makeModel(scope: "initial", chatIDs: [], agents: [], presence: presence)
        model.selection.selectedIDs = ["team", "family"]
        model.saveScope(all: false)
        try await waitUntil { (try? fixture.requests().contains("PUT /scope")) == true }
        model.saveScope(all: true)
        XCTAssertTrue(presence.reasons.isEmpty)
    }

    func testWideningScopeRequiresPresence() async throws {
        let presence = FakePresence(result: false)
        let (model, fixture) = try await makeModel(scope: "selected", chatIDs: ["team"], presence: presence)
        model.selection.selectedIDs = ["team", "other-chat"]
        model.saveScope(all: false)
        try await waitUntil { model.error != nil }
        model.error = nil
        model.saveScope(all: true)
        try await waitUntil { model.error != nil }
        XCTAssertEqual(presence.reasons, Array(repeating: "share more WhatsApp chats with connected agents", count: 2))
        XCTAssertFalse(try fixture.requests().contains("PUT /scope"))
    }

    func testNarrowingScopeNeverPrompts() async throws {
        let selected = FakePresence(result: false)
        let (model, fixture) = try await makeModel(scope: "selected", chatIDs: ["team", "other-chat"], presence: selected)
        model.selection.selectedIDs = ["team"]
        model.saveScope(all: false)
        try await waitUntil { (try? fixture.requests().contains("PUT /scope")) == true && !model.busy }
        XCTAssertTrue(selected.reasons.isEmpty)

        let all = FakePresence(result: false)
        let (allModel, allFixture) = try await makeModel(scope: "all", chatIDs: [], presence: all)
        allModel.selection.selectedIDs = ["team"]
        allModel.saveScope(all: false)
        try await waitUntil { (try? allFixture.requests().contains("PUT /scope")) == true && !allModel.busy }
        XCTAssertTrue(all.reasons.isEmpty)
    }

    func testRevealingSavedTokenRequiresPresenceAndDoesNotRetryOnPoll() async throws {
        let presence = FakePresence(result: false)
        let (model, fixture) = try await makeModel(presence: presence)
        let credentials = "GET /agents/first/credentials"
        model.toggleLocalMCP()
        try await waitUntil { presence.reasons.count == 1 && model.error != nil }
        XCTAssertEqual(presence.reasons, ["show the access token for Connection 1"])
        await model.refresh()
        model.reconcileConnections()
        await model.refresh()
        XCTAssertEqual(presence.reasons.count, 1, "Polling must not repeat a denied confirmation")
        XCTAssertFalse(try fixture.requests().contains(credentials))
        XCTAssertNil(model.connectionCredentials)

        model.toggleLocalMCP()
        model.toggleLocalMCP()
        try await waitUntil { presence.reasons.count == 2 }
        XCTAssertFalse(try fixture.requests().contains(credentials))

        presence.result = true
        model.toggleLocalMCP()
        model.toggleLocalMCP()
        try await waitUntil { model.connectionCredentials != nil && !model.busy }
        XCTAssertEqual(model.connectionCredentials?.accessToken, "SYNTHETIC-PRESENCE-CREDENTIAL")
        XCTAssertEqual(try fixture.requests().filter { $0 == credentials }.count, 1)
    }

    func testFirstConnectionNeverPrompts() async throws {
        let presence = FakePresence(result: false)
        let (model, fixture) = try await makeModel(agents: [], presence: presence)
        model.beginPairing()
        model.copyInstructions()
        try await waitUntil { (try? fixture.requests().contains("POST /pairing")) == true && !model.busy }
        model.toggleManualSetup()
        try await waitUntil { (try? fixture.requests().contains("POST /agents/manual")) == true && !model.busy }
        XCTAssertTrue(presence.reasons.isEmpty)
    }

    func testAdditionalConnectionRequiresPresence() async throws {
        let presence = FakePresence(result: false)
        let (model, fixture) = try await makeModel(agents: [Self.first], presence: presence)
        let reason = "connect another agent to your WhatsApp chats"
        model.beginPairing()
        model.copyInstructions()
        try await waitUntil { presence.reasons.count == 1 && model.error != nil }
        model.error = nil
        model.toggleManualSetup()
        try await waitUntil { presence.reasons.count == 2 && model.error != nil }
        XCTAssertEqual(presence.reasons, [reason, reason])
        XCTAssertFalse(try fixture.requests().contains { $0.hasPrefix("POST ") })

        presence.result = true
        model.copyInstructions()
        try await waitUntil { (try? fixture.requests().contains("POST /pairing")) == true && !model.busy }
        model.toggleManualSetup()
        model.toggleManualSetup()
        try await waitUntil { (try? fixture.requests().contains("POST /agents/manual")) == true && !model.busy }
        XCTAssertEqual(presence.reasons.count, 3, "One approval covers both creations inside the reuse window")
    }

    func testPresenceReuseWindow() async throws {
        let presence = FakePresence(result: true), clock = TestClock()
        let (model, fixture) = try await makeModel(presence: presence, now: { clock.current })
        let enable = { model.changeSending(try self.agent("first", in: model), enabled: true) }
        let patches = { (try? fixture.requests().filter { $0 == "PATCH /agents/first" }.count) ?? 0 }

        try enable()
        try await waitUntil { patches() == 1 && !model.busy }
        clock.current += 30
        try enable()
        try await waitUntil { patches() == 2 && !model.busy }
        XCTAssertEqual(presence.reasons.count, 1, "A second action within 60 s reuses the confirmation")

        clock.current += 31
        try enable()
        try await waitUntil { patches() == 3 && !model.busy }
        XCTAssertEqual(presence.reasons.count, 2, "The confirmation expires after 60 s")

        model.panelClosed()
        try enable()
        try await waitUntil { patches() == 4 && !model.busy }
        XCTAssertEqual(presence.reasons.count, 3, "Closing the panel clears the confirmation")
    }

    func testDemoNeverPrompts() throws {
        let presence = FakePresence(result: false)
        let pasteboard = NSPasteboard(name: .init("LocalWhatsAppMCP-PresenceTests-\(UUID().uuidString)"))
        addTeardownBlock { @MainActor in pasteboard.releaseGlobally() }
        let model = AppModel(environment: ["LWMCP_DEMO": "1", "LWMCP_DEMO_PAGE": "mcp-agents"], pasteboard: pasteboard, presence: presence)
        model.page = .home
        let agent = try XCTUnwrap(model.homeAgent)
        model.changeSending(agent, enabled: true)
        XCTAssertEqual(model.state.agents.first { $0.id == agent.id }?.sendEnabled, true)
        model.toggleLocalMCP()
        XCTAssertNotNil(model.connectionCredentials)
        model.saveScope(all: true)
        XCTAssertEqual(model.state.scope.mode, "all")
        XCTAssertTrue(presence.reasons.isEmpty)
        XCTAssertNil(model.error)
    }

    private func agent(_ id: String, in model: AppModel) throws -> AgentConnection {
        try XCTUnwrap(model.state.agents.first { $0.id == id })
    }

    private func makeModel(scope: String = "selected", chatIDs: [String] = ["team"], agents: [[String: Any]]? = nil,
                           presence: FakePresence, now: @escaping () -> Date = Date.init) async throws -> (AppModel, PresenceFixture) {
        let fixture = try PresenceFixture(scope: scope, chatIDs: chatIDs, agents: agents ?? [Self.first, Self.other])
        let pasteboard = NSPasteboard(name: .init("LocalWhatsAppMCP-PresenceTests-\(UUID().uuidString)"))
        let preferencesName = "LocalWhatsAppMCP-PresenceTests-\(UUID().uuidString)"
        let preferences = try XCTUnwrap(UserDefaults(suiteName: preferencesName))
        let model = AppModel(environment: [:], pasteboard: pasteboard, runtime: fixture.runtime, preferences: preferences, presence: presence, now: now)
        addTeardownBlock { @MainActor in
            await model.stop()
            preferences.removePersistentDomain(forName: preferencesName)
            pasteboard.releaseGlobally()
            try? FileManager.default.removeItem(at: fixture.directory)
        }
        await model.refresh()
        XCTAssertTrue(model.serviceAvailable)
        return (model, fixture)
    }

    private func waitUntil(file: StaticString = #filePath, line: UInt = #line, _ condition: () -> Bool) async throws {
        for _ in 0..<500 {
            if condition() { return }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTFail("Timed out waiting for the presence fixture", file: file, line: line)
        throw PresenceFixtureFailure.timedOut
    }
}

private enum PresenceFixtureFailure: Error { case timedOut }

/// Runs the production runtime/control path against disposable synthetic JSON.
private struct PresenceFixture {
    let directory: URL
    let runtime: HelperRuntime

    init(scope: String, chatIDs: [String], agents: [[String: Any]]) throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent("LocalWhatsAppMCP-Presence-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let helper = directory.appendingPathComponent("helper")
        runtime = HelperRuntime(helper: helper, wacli: helper, dataDirectory: directory)
        let state: [String: Any] = [
            "whatsapp": ["linked": true, "status": "ready"],
            "scope": ["mode": scope, "chat_ids": chatIDs],
            "agents": agents,
            "mcp": ["url": "http://127.0.0.1:32191/mcp", "status": scope == "initial" ? "setup_required" : "ready"]
        ]
        let expiry = ISO8601DateFormatter().string(from: Date().addingTimeInterval(600))
        try write(state, to: "state.json")
        try write(["instructions": "Synthetic setup instructions.", "expires_at": expiry, "invitation_id": "synthetic-invitation"], to: "pairing.json")
        try write(["id": "manual", "name": "Connection 3", "url": "http://127.0.0.1:32191/mcp", "access_token": "SYNTHETIC-MANUAL-CREDENTIAL"], to: "manual.json")
        try write(["id": "first", "name": "Connection 1", "url": "http://127.0.0.1:32191/mcp", "access_token": "SYNTHETIC-PRESENCE-CREDENTIAL"], to: "credentials.json")
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
        IFS= read -r _session_token
        /usr/bin/printf '%s %s\\n' "$method" "$request_path" >> "$fixture_dir/requests.log"
        case "$method $request_path" in
          'GET /state'|'PUT /scope'|'PATCH /agents/'*) /bin/cat "$fixture_dir/state.json" ;;
          'GET /catalog') /usr/bin/printf '%s' '{"chats":[]}' ;;
          'POST /pairing') /bin/cat "$fixture_dir/pairing.json" ;;
          'POST /agents/manual') /bin/cat "$fixture_dir/manual.json" ;;
          'GET /agents/first/credentials') /bin/cat "$fixture_dir/credentials.json" ;;
          *) exit 1 ;;
        esac
        """.utf8).write(to: helper)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: helper.path)
    }

    private func write(_ value: [String: Any], to name: String) throws {
        try JSONSerialization.data(withJSONObject: value).write(to: directory.appendingPathComponent(name), options: .atomic)
    }
    func requests() throws -> [String] {
        let log = directory.appendingPathComponent("requests.log")
        guard FileManager.default.fileExists(atPath: log.path) else { return [] }
        return try String(contentsOf: log, encoding: .utf8).split(separator: "\n").map(String.init)
    }
}
