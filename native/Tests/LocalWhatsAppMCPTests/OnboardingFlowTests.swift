import AppKit
import XCTest
import LocalWhatsAppCore
@testable import LocalWhatsAppMCP

@MainActor
final class OnboardingFlowTests: XCTestCase {
    func testInitialLinkOpensChooserThenSelectedScopeOpensAgentSetup() async throws {
        let (model, fixture) = try makeModel(state: serviceState(linked: false, status: "disconnected"))
        model.panelOpened()
        await model.refresh()
        try await waitUntil { model.page == .qr && !model.busy && fixture.exists("connect-requested") }
        XCTAssertFalse(model.state.scope.isConfirmed)

        try fixture.write(serviceState(), to: "state.json")
        try fixture.hold("catalog")
        let refresh = Task { await model.refresh() }
        try await waitUntil { model.page == .chats && fixture.exists("catalog-requested") }
        XCTAssertTrue(model.catalogPending)
        XCTAssertFalse(model.canCreateConnection)
        fixture.release("catalog")
        await refresh.value
        XCTAssertFalse(model.catalogPending)
        XCTAssertEqual(Set(model.selection.chats.map(\.id)), ["team", "family"])

        model.selection.selectedIDs = ["team"]
        try fixture.write(serviceState(scope: "selected", chatIDs: ["team"]), to: "scope-response.json")
        model.saveScope(all: false)
        try await waitUntil { !model.busy && model.state.scope.isConfirmed }
        XCTAssertEqual(model.page, .mcp)
        XCTAssertTrue(model.agents.isEmpty, "The empty Local MCP route renders Connect your agent")
        XCTAssertTrue(model.canCreateConnection)
        XCTAssertEqual(try fixture.scopeRequest()["mode"] as? String, "selected")
        XCTAssertEqual(try fixture.scopeRequest()["chat_ids"] as? [String], ["team"])

        model.page = .home
        await model.refresh()
        XCTAssertEqual(model.page, .home, "Exiting agent setup must survive polling")
    }

    func testInitialAllChatsConfirmationAlsoOpensAgentSetup() async throws {
        let (model, fixture) = try makeModel()
        await model.refresh()
        XCTAssertEqual(model.page, .chats)
        try fixture.write(serviceState(scope: "all"), to: "scope-response.json")
        model.saveScope(all: true)
        try await waitUntil { !model.busy && model.state.scope.isConfirmed }
        XCTAssertEqual(model.page, .mcp)
        XCTAssertEqual(model.state.scope.mode, "all")
        XCTAssertTrue(model.canCreateConnection)
        XCTAssertEqual(try fixture.scopeRequest()["mode"] as? String, "all")
        XCTAssertEqual(try fixture.scopeRequest()["chat_ids"] as? [String], [])
    }

    func testSyntheticPreviewUsesTheSameInitialSetupHandoffs() {
        let pasteboard = NSPasteboard(name: .init("LocalWhatsAppMCP-OnboardingTests-\(UUID().uuidString)"))
        defer { pasteboard.releaseGlobally() }
        let model = AppModel(environment: ["LWMCP_DEMO": "1", "LWMCP_DEMO_PAGE": "initial-linked"], pasteboard: pasteboard)
        XCTAssertEqual(model.page, .chats)
        XCTAssertFalse(model.canCreateConnection)
        model.selection.selectedIDs = ["work"]
        model.saveScope(all: false)
        XCTAssertEqual(model.page, .mcp)
        XCTAssertTrue(model.canCreateConnection)
    }

    func testIncompleteSavedLinkResumesChooserOnlyOnceAndAllowsNormalNavigation() async throws {
        let (model, _) = try makeModel(state: serviceState(status: "attention"))
        await model.refresh()
        XCTAssertEqual(model.page, .chats, "A saved authenticated account can resume chat selection while offline")

        for destination in [AppModel.Page.home, .settings, .mcp] {
            model.page = destination
            await model.refresh()
            model.panelOpened()
            await model.refresh()
            XCTAssertEqual(model.page, destination)
        }
        model.page = .home
        model.editChats()
        await model.refresh()
        XCTAssertEqual(model.page, .chats, "The overview's Choose chats action can resume setup explicitly")
    }

    func testConfirmedReturningUserKeepsNormalNavigationWithoutSavedAgents() async throws {
        let (model, _) = try makeModel(state: serviceState(status: "connecting", scope: "selected", chatIDs: ["team"]))
        await model.refresh()
        XCTAssertEqual(model.page, .home)
        XCTAssertTrue(model.agents.isEmpty)
        for destination in [AppModel.Page.settings, .mcp, .home] {
            model.page = destination
            model.panelOpened()
            await model.refresh()
            XCTAssertEqual(model.page, destination)
        }
    }

    func testExplicitSettingsBeforeFirstResponseIsNotReplacedByOnboarding() async throws {
        let (model, _) = try makeModel()
        model.page = .settings
        await model.refresh()
        XCTAssertEqual(model.page, .settings)
        model.page = .home
        await model.refresh()
        XCTAssertEqual(model.page, .home)
    }

    func testChooserDraftAndSearchSurvivePollingAndWindowReopen() async throws {
        let (model, _) = try makeModel()
        await model.refresh()
        model.selection.selectedIDs = ["family"]
        model.selection.query = "Family"
        await model.refresh()
        model.panelClosed()
        model.panelOpened()
        await model.refresh()
        XCTAssertEqual(model.page, .chats)
        XCTAssertEqual(model.selection.selectedIDs, ["family"])
        XCTAssertEqual(model.selection.query, "Family")
        XCTAssertFalse(model.state.scope.isConfirmed)
    }

    func testLeavingQRDuringLinkingPreventsSuccessfulLinkFromTakingOverNavigation() async throws {
        for destination in [AppModel.Page.home, .settings] {
            let (model, fixture) = try makeModel(state: serviceState(linked: false, status: "linking"))
            model.page = .qr
            await model.refresh()
            model.page = destination
            try fixture.write(serviceState(), to: "state.json")
            await model.refresh()
            XCTAssertEqual(model.page, destination)
            XCTAssertFalse(model.state.scope.isConfirmed)
        }
    }

    func testConfirmedRelinkReturnsToOverviewAndSubsequentScopeEditReturnsHome() async throws {
        let (model, fixture) = try makeModel(state: serviceState(status: "connecting", scope: "selected", chatIDs: ["team"]))
        await model.refresh()
        model.page = .qr
        try fixture.write(serviceState(scope: "selected", chatIDs: ["team"]), to: "state.json")
        await model.refresh()
        XCTAssertEqual(model.page, .home)

        model.editChats()
        await model.refresh()
        XCTAssertEqual(model.selection.selectedIDs, ["team"])
        model.selection.selectedIDs = ["family"]
        try fixture.write(serviceState(scope: "selected", chatIDs: ["family"]), to: "scope-response.json")
        model.saveScope(all: false)
        try await waitUntil { !model.busy && model.state.scope.chatIDs == ["family"] }
        XCTAssertEqual(model.page, .home)
        XCTAssertTrue(model.agents.isEmpty, "Editing confirmed scope must not restart agent onboarding")
    }

    func testFailedInitialScopeSaveKeepsChooserAndDraftWithoutOpeningAgentSetup() async throws {
        let (model, fixture) = try makeModel()
        await model.refresh()
        model.selection.selectedIDs = ["team"]
        model.selection.query = "Product"
        try fixture.write(["error": "Synthetic scope save failed"], to: "scope-failure.json")
        model.saveScope(all: false)
        try await waitUntil { !model.busy && model.error != nil }
        XCTAssertEqual(model.page, .chats)
        XCTAssertFalse(model.state.scope.isConfirmed)
        XCTAssertEqual(model.selection.selectedIDs, ["team"])
        XCTAssertEqual(model.selection.query, "Product")
        XCTAssertEqual(model.error, "Synthetic scope save failed")
    }

    func testExitingWhileInitialScopeSaveIsPendingPreventsLateAgentSetupNavigation() async throws {
        let (model, fixture) = try makeModel()
        await model.refresh()
        model.selection.selectedIDs = ["team"]
        try fixture.write(serviceState(scope: "selected", chatIDs: ["team"]), to: "scope-response.json")
        try fixture.hold("scope")
        model.saveScope(all: false)
        try await waitUntil { fixture.exists("scope-requested") }
        model.page = .settings
        fixture.release("scope")
        try await waitUntil { !model.busy }
        XCTAssertTrue(model.state.scope.isConfirmed)
        XCTAssertEqual(model.page, .settings)
        await model.refresh()
        XCTAssertEqual(model.page, .settings)
    }

    func testLeavingAndReenteringChooserWhileSavingPreservesNewNavigationAndDraft() async throws {
        let (model, fixture) = try makeModel()
        await model.refresh()
        model.selection.selectedIDs = ["team"]
        try fixture.write(serviceState(scope: "selected", chatIDs: ["team"]), to: "scope-response.json")
        try fixture.hold("scope")
        model.saveScope(all: false)
        try await waitUntil { fixture.exists("scope-requested") }
        model.page = .home
        model.editChats()
        model.selection.selectedIDs = ["family"]
        model.selection.query = "Family"
        fixture.release("scope")
        try await waitUntil { !model.busy }
        XCTAssertEqual(model.page, .chats)
        XCTAssertEqual(model.selection.selectedIDs, ["family"])
        XCTAssertEqual(model.selection.query, "Family")
    }

    private func waitUntil(file: StaticString = #filePath, line: UInt = #line, _ condition: () -> Bool) async throws {
        for _ in 0..<500 {
            if condition() { return }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTFail("Timed out waiting for the synthetic service", file: file, line: line)
        throw FixtureFailure.timedOut
    }

    private func serviceState(linked: Bool = true, status: String = "syncing", scope: String = "initial", chatIDs: [String] = []) -> [String: Any] {
        ["whatsapp": ["linked": linked, "status": status],
         "scope": ["mode": scope, "chat_ids": chatIDs],
         "agents": [],
         "mcp": ["url": "http://127.0.0.1:32191/mcp", "status": scope == "initial" ? "setup_required" : "ready"]]
    }

    private func makeModel(state: [String: Any]? = nil) throws -> (AppModel, ServiceFixture) {
        let fixture = try ServiceFixture(state: state ?? serviceState())
        let pasteboard = NSPasteboard(name: .init("LocalWhatsAppMCP-OnboardingTests-\(UUID().uuidString)"))
        let model = AppModel(environment: [:], pasteboard: pasteboard, runtime: fixture.runtime)
        addTeardownBlock { @MainActor in
            fixture.release("scope")
            fixture.release("catalog")
            await model.stop()
            pasteboard.releaseGlobally()
            try? FileManager.default.removeItem(at: fixture.directory)
        }
        return (model, fixture)
    }
}

private enum FixtureFailure: Error { case timedOut }

/// Executes the actual runtime/control path with only disposable JSON fixtures.
private struct ServiceFixture {
    let directory: URL
    let runtime: HelperRuntime

    init(state: [String: Any]) throws {
        directory = FileManager.default.temporaryDirectory.appendingPathComponent("LocalWhatsAppMCP-Onboarding-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let helper = directory.appendingPathComponent("helper")
        runtime = HelperRuntime(helper: helper, wacli: helper, dataDirectory: directory)
        try write(state, to: "state.json")
        try write(["whatsapp": ["linked": false, "status": "linking", "qr": "SYNTHETIC-NOT-A-PAIRING-CODE"], "scope": ["mode": "initial"], "agents": [], "mcp": ["status": "setup_required"]], to: "connect-response.json")
        try write(["chats": [["id": "team", "name": "Product team", "kind": "group"], ["id": "family", "name": "Family", "kind": "group"]]], to: "catalog.json")
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
        case "$method $request_path" in
          'GET /state') /bin/cat "$fixture_dir/state.json" ;;
          'GET /catalog')
            /usr/bin/touch "$fixture_dir/catalog-requested"
            while [ -e "$fixture_dir/catalog-hold" ]; do /bin/sleep 0.01; done
            /bin/cat "$fixture_dir/catalog.json"
            ;;
          'POST /whatsapp/connect')
            /usr/bin/touch "$fixture_dir/connect-requested"
            /bin/cp "$fixture_dir/connect-response.json" "$fixture_dir/state.json"
            /bin/cat "$fixture_dir/state.json"
            ;;
          'PUT /scope')
            /bin/cat > "$fixture_dir/scope-request.json"
            /usr/bin/touch "$fixture_dir/scope-requested"
            while [ -e "$fixture_dir/scope-hold" ]; do /bin/sleep 0.01; done
            if [ -e "$fixture_dir/scope-failure.json" ]; then
              /bin/cat "$fixture_dir/scope-failure.json"
              exit 1
            fi
            /bin/cp "$fixture_dir/scope-response.json" "$fixture_dir/state.json"
            /bin/cat "$fixture_dir/state.json"
            ;;
          *) exit 1 ;;
        esac
        """.utf8).write(to: helper)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: helper.path)
    }

    func write(_ value: [String: Any], to name: String) throws {
        try JSONSerialization.data(withJSONObject: value).write(to: directory.appendingPathComponent(name), options: .atomic)
    }
    func exists(_ name: String) -> Bool { FileManager.default.fileExists(atPath: directory.appendingPathComponent(name).path) }
    func hold(_ operation: String) throws { try Data().write(to: directory.appendingPathComponent("\(operation)-hold")) }
    func release(_ operation: String) { try? FileManager.default.removeItem(at: directory.appendingPathComponent("\(operation)-hold")) }
    func scopeRequest() throws -> [String: Any] {
        try XCTUnwrap(JSONSerialization.jsonObject(with: Data(contentsOf: directory.appendingPathComponent("scope-request.json"))) as? [String: Any])
    }
}
