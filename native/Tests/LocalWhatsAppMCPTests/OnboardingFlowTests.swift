import AppKit
import XCTest
import LocalWhatsAppCore
@testable import LocalWhatsAppMCP

@MainActor
final class OnboardingFlowTests: XCTestCase {
    func testConnectedServiceNeedsNoSyncMetadata() async throws {
        let (model, _) = try makeModel(state: [
            "whatsapp": ["linked": true, "status": "connected"],
            "scope": ["mode": "all"],
            "agents": [],
            "mcp": ["url": "http://127.0.0.1:32191/mcp", "status": "ready"]
        ])
        await model.refresh()
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 2 chats connected")
        XCTAssertNil(model.connectionError)
        XCTAssertTrue(model.canCreateConnection)
    }

    func testLegacySyncMetadataCannotOverrideConnectionState() async throws {
        let (model, fixture) = try makeModel()
        for oldState in ["caught_up", "syncing", "incomplete", "error", "unknown"] {
            var response = serviceState(status: "ready", scope: "all")
            response["whatsapp"] = ["linked": true, "status": "ready", "sync_state": oldState,
                                    "last_synced_at": "2026-10-01T12:00:00Z"]
            try fixture.write(response, to: "state.json")
            await model.refresh()
            XCTAssertEqual(model.statusLabel, "Connected to this Mac")
            XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 2 chats connected")
            XCTAssertNil(model.connectionError)
        }
    }

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

    func testTerminalLogoutAfterScanStaysVisibleUntilExplicitRetry() async throws {
        let logoutDetail = "WhatsApp disconnected this linked device. Connect WhatsApp again."
        let (model, fixture) = try makeModel(state: serviceState(linked: false, status: "linking", qr: "SYNTHETIC-INITIAL-QR"))
        model.page = .qr
        await model.refresh()
        XCTAssertEqual(model.statusLabel, "Waiting for your phone")
        XCTAssertNil(model.connectionError)

        // The phone accepting pairing is not proof of a surviving authenticated
        // connection. A terminal logout before the next poll must remain visible.
        try fixture.write(serviceState(linked: false, status: "attention", detail: logoutDetail), to: "state.json")
        for _ in 0..<3 { await model.refresh() }
        XCTAssertEqual(model.page, .qr)
        XCTAssertFalse(model.whatsappLinked)
        XCTAssertEqual(model.connectionError, logoutDetail)
        XCTAssertTrue(model.whatsappNeedsReconnect)
        XCTAssertNil(model.state.whatsapp.qr)
        XCTAssertFalse(fixture.exists("connect-requested"), "Background polling must not repeatedly relink a revoked session")
        XCTAssertFalse(model.canCreateConnection)

        model.connectWhatsApp()
        try await waitUntil { !model.busy && fixture.exists("connect-requested") }
        XCTAssertEqual(model.page, .qr)
        XCTAssertEqual(model.statusLabel, "Waiting for your phone")
        XCTAssertNil(model.connectionError)
        XCTAssertNotNil(model.state.whatsapp.qr)
        XCTAssertFalse(model.whatsappLinked)
        XCTAssertFalse(model.state.scope.isConfirmed)

        try fixture.write(serviceState(), to: "state.json")
        await model.refresh()
        XCTAssertEqual(model.page, .chats)
        XCTAssertTrue(model.whatsappLinked)
        XCTAssertFalse(model.canCreateConnection, "Relinking must still require the initial chat-scope confirmation")
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac")
    }

    func testDelayedTerminalLogoutPollCannotReplaceFreshRetryQRCode() async throws {
        let logoutDetail = "WhatsApp disconnected this linked device. Connect WhatsApp again."
        let (model, fixture) = try makeModel(state: serviceState(linked: false, status: "attention", detail: logoutDetail))
        model.page = .qr
        await model.refresh()
        XCTAssertEqual(model.connectionError, logoutDetail)

        try fixture.hold("state")
        fixture.remove("state-requested")
        let stalePoll = Task { await model.refresh() }
        try await waitUntil { fixture.exists("state-requested") }
        model.connectWhatsApp()
        try await waitUntil { !model.busy && fixture.exists("connect-requested") }
        let retryQRCode = try XCTUnwrap(model.state.whatsapp.qr)
        fixture.release("state")
        await stalePoll.value

        XCTAssertEqual(model.state.whatsapp.qr, retryQRCode)
        XCTAssertEqual(model.statusLabel, "Waiting for your phone")
        XCTAssertNil(model.connectionError)
        XCTAssertFalse(model.whatsappNeedsReconnect)
        XCTAssertFalse(model.whatsappLinked)
        XCTAssertEqual(model.page, .qr)
    }

    func testTemporaryDisconnectAfterScanKeepsSavedLinkWithoutRelinking() async throws {
        let (model, fixture) = try makeModel(state: serviceState(linked: false, status: "linking", qr: "SYNTHETIC-INITIAL-QR"))
        model.page = .qr
        await model.refresh()
        try fixture.write(serviceState(), to: "state.json")
        await model.refresh()
        XCTAssertEqual(model.page, .chats)

        try fixture.write(serviceState(status: "connecting"), to: "state.json")
        await model.refresh()
        XCTAssertEqual(model.page, .chats)
        XCTAssertTrue(model.whatsappLinked)
        XCTAssertFalse(model.whatsappConnected)
        XCTAssertFalse(model.whatsappNeedsReconnect)
        XCTAssertEqual(model.homeSubtitle, "Reconnecting…")
        XCTAssertNil(model.connectionError)
        XCTAssertFalse(fixture.exists("connect-requested"))

        try fixture.write(serviceState(status: "ready"), to: "state.json")
        await model.refresh()
        XCTAssertEqual(model.page, .chats)
        XCTAssertTrue(model.whatsappConnected)
        XCTAssertNil(model.connectionError)
        XCTAssertFalse(model.state.scope.isConfirmed)
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

    func testConnectionCountUsesSavedScopeAndVerifiedAliases() async throws {
        let (model, fixture) = try makeModel(state: serviceState(scope: "selected", chatIDs: ["1@s.whatsapp.net", "2@lid"]))
        try fixture.write(["chats": [
            ["id": "1@s.whatsapp.net", "aliases": ["1@s.whatsapp.net", "2@lid"]],
            ["id": "2@lid", "aliases": ["1@s.whatsapp.net", "2@lid"]],
            ["id": "group@g.us"]
        ]], to: "catalog.json")
        XCTAssertEqual(model.connectedChatsSummary, "Loading connected chats…")
        await model.refresh()
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 1 chat connected")
        model.selection.selectedIDs = ["group@g.us"]
        model.selection.query = "No matching name"
        XCTAssertEqual(model.connectedChatsSummary, "1 chat connected")

        try fixture.write(serviceState(scope: "all"), to: "state.json")
        await model.refresh()
        XCTAssertEqual(model.connectedChatsSummary, "2 chats connected")
        try fixture.write(serviceState(scope: "selected"), to: "state.json")
        await model.refresh()
        XCTAssertEqual(model.connectedChatsSummary, "0 chats connected")
    }

    func testIntentionalDisconnectStaysDisconnectedDuringBackgroundPolling() async throws {
        let (model, fixture) = try makeModel(state: serviceState(scope: "all"))
        await model.refresh()
        try fixture.write(serviceState(linked: false, status: "disconnected", scope: "all"), to: "disconnect-response.json")
        try fixture.hold("disconnect")
        model.disconnectWhatsApp()
        XCTAssertEqual(model.homeSubtitle, "Disconnecting…")
        try await waitUntil { fixture.exists("disconnect-requested") }
        await model.refresh()
        XCTAssertEqual(model.homeSubtitle, "Disconnecting…")
        fixture.release("disconnect")
        try await waitUntil { !model.busy }
        await model.refresh()
        XCTAssertEqual(model.homeSubtitle, "Not connected")
        XCTAssertFalse(model.whatsappLinked)
        XCTAssertEqual(model.page, .home)
        XCTAssertNil(model.connectionError)
        XCTAssertFalse(fixture.exists("connect-requested"))
    }

    func testConnectionPreviewAndActualFailures() {
        let pasteboard = NSPasteboard(name: .init("LocalWhatsAppMCP-ConnectionTests-\(UUID().uuidString)"))
        defer { pasteboard.releaseGlobally() }
        let model = AppModel(environment: ["LWMCP_DEMO": "1", "LWMCP_DEMO_PAGE": "connected"], pasteboard: pasteboard)
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 1,292 chats connected")
        for (status, label) in [("connecting", "Reconnecting…"), ("attention", "Needs attention"), ("disconnected", "Connection paused")] {
            model.state.whatsapp.status = status
            XCTAssertEqual(model.homeSubtitle, label)
        }
    }

    func testActualConnectionFailureRemainsVisibleAndClearsWhenConnected() async throws {
        let failureDetail = "The WhatsApp archive could not be opened. Check available storage."
        let (model, fixture) = try makeModel(state: serviceState(status: "attention", scope: "all", detail: failureDetail))
        await model.refresh()
        XCTAssertEqual(model.homeSubtitle, "Needs attention")
        XCTAssertEqual(model.connectionError, failureDetail)
        XCTAssertTrue(model.whatsappNeedsReconnect)

        try fixture.write(serviceState(scope: "all"), to: "state.json")
        await model.refresh()
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 2 chats connected")
        XCTAssertNil(model.connectionError)
        XCTAssertFalse(model.whatsappNeedsReconnect)
    }

    func testWakeShowsReconnectImmediatelyAndRejectsEarlierPoll() async throws {
        let connected = serviceState(scope: "all")
        let (model, fixture) = try makeModel(state: connected)
        await model.refresh()
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 2 chats connected")
        try fixture.hold("state")
        fixture.remove("state-requested")
        let earlierPoll = Task { await model.refresh() }
        try await waitUntil { fixture.exists("state-requested") }
        try fixture.hold("wake")
        try fixture.write(serviceState(status: "connecting", scope: "all"), to: "wake-response.json")
        model.wake()
        XCTAssertEqual(model.homeSubtitle, "Reconnecting…")
        try await waitUntil { fixture.exists("wake-requested") }
        fixture.release("state")
        await earlierPoll.value
        XCTAssertEqual(model.homeSubtitle, "Reconnecting…")
        fixture.release("wake")
        try await waitUntil { !model.busy }
        XCTAssertEqual(model.homeSubtitle, "Reconnecting…")
        try fixture.write(connected, to: "state.json")
        await model.refresh()
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 2 chats connected")
    }

    func testReconnectCannotBeOverwrittenByPollDuringAction() async throws {
        let (model, fixture) = try makeModel(state: serviceState(scope: "all"))
        await model.refresh()
        try fixture.hold("connect")
        model.connectWhatsApp()
        XCTAssertEqual(model.homeSubtitle, "Reconnecting…")
        try await waitUntil { fixture.exists("connect-requested") }
        await model.refresh()
        XCTAssertEqual(model.homeSubtitle, "Reconnecting…")
        fixture.release("connect")
        try await waitUntil { !model.busy }
        XCTAssertEqual(model.homeSubtitle, "Waiting for your phone")
        XCTAssertFalse(model.whatsappLinked)
        XCTAssertNotNil(model.state.whatsapp.qr)
    }

    func testScopeSaveKeepsConnectionAndUsesSavedChatCount() async throws {
        let (model, fixture) = try makeModel(state: serviceState(scope: "all"))
        await model.refresh()
        model.selection.selectedIDs = ["team"]
        model.selection.query = "Family"
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 2 chats connected")
        try fixture.write(serviceState(scope: "selected", chatIDs: ["team"]), to: "scope-response.json")
        try fixture.hold("scope")
        model.saveScope(all: false)
        try await waitUntil { fixture.exists("scope-requested") }
        await model.refresh()
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 2 chats connected")
        fixture.release("scope")
        try await waitUntil { !model.busy }
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 1 chat connected")
        XCTAssertNil(model.connectionError)
    }

    func testStateReadFailureHidesConnectionButCatalogFailureDoesNotDisconnect() async throws {
        let (model, fixture) = try makeModel(state: serviceState(scope: "all"))
        await model.refresh()
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 2 chats connected")
        fixture.remove("catalog.json")
        model.editChats()
        await model.refresh()
        try await waitUntil { model.error != nil }
        XCTAssertTrue(model.serviceAvailable)
        XCTAssertEqual(model.homeSubtitle, "Connected to this Mac - 2 chats connected")
        fixture.remove("state.json")
        await model.refresh()
        XCTAssertFalse(model.serviceAvailable)
        XCTAssertEqual(model.homeSubtitle, "Reconnecting…")
    }

    func testWakeQueuedDuringScopeSaveStillRuns() async throws {
        let connected = serviceState(scope: "all")
        let (model, fixture) = try makeModel(state: connected)
        await model.refresh()
        try fixture.write(connected, to: "scope-response.json")
        try fixture.write(serviceState(status: "connecting", scope: "all"), to: "wake-response.json")
        try fixture.hold("scope")
        try fixture.hold("wake")
        model.saveScope(all: true)
        try await waitUntil { fixture.exists("scope-requested") }
        model.wake()
        fixture.release("scope")
        try await waitUntil { fixture.exists("wake-requested") }
        XCTAssertEqual(model.homeSubtitle, "Reconnecting…")
        fixture.release("wake")
        try await waitUntil { !model.busy }
        XCTAssertEqual(model.homeSubtitle, "Reconnecting…")
    }

    func testFailedMutationRefreshesStateAfterClearingBusy() async throws {
        let (model, fixture) = try makeModel(state: serviceState(scope: "all"))
        await model.refresh()
        try fixture.write(["error": "Synthetic scope save failed"], to: "scope-failure.json")
        try fixture.write(serviceState(status: "connecting", scope: "all"), to: "state.json")
        model.saveScope(all: false)
        try await waitUntil { !model.busy && model.state.whatsapp.status == "connecting" }
        XCTAssertEqual(model.error, "Synthetic scope save failed")
    }

    func testWakeQueuedDuringPairingStillRuns() async throws {
        let (model, fixture) = try makeModel(state: serviceState(scope: "all"))
        await model.refresh()
        model.page = .mcp
        try fixture.write(["instructions": "Synthetic setup instructions", "expires_at": ISO8601DateFormatter().string(from: Date().addingTimeInterval(600)), "invitation_id": "synthetic"], to: "pairing-response.json")
        try fixture.write(serviceState(status: "connecting", scope: "all"), to: "wake-response.json")
        try fixture.hold("pairing")
        model.copyInstructions()
        try await waitUntil { fixture.exists("pairing-requested") }
        model.wake()
        fixture.release("pairing")
        try await waitUntil { fixture.exists("wake-requested") && !model.busy }
        XCTAssertEqual(model.homeSubtitle, "Reconnecting…")
    }

    private func waitUntil(file: StaticString = #filePath, line: UInt = #line, _ condition: () -> Bool) async throws {
        for _ in 0..<500 {
            if condition() { return }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTFail("Timed out waiting for the synthetic service", file: file, line: line)
        throw FixtureFailure.timedOut
    }

    private func serviceState(linked: Bool = true, status: String = "ready", scope: String = "initial", chatIDs: [String] = [], detail: String? = nil, qr: String? = nil) -> [String: Any] {
        var whatsapp: [String: Any] = ["linked": linked, "status": status]
        if let detail { whatsapp["detail"] = detail }
        if let qr { whatsapp["qr"] = qr }
        return ["whatsapp": whatsapp,
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
            fixture.release("state")
            fixture.release("wake")
            fixture.release("connect")
            fixture.release("disconnect")
            fixture.release("pairing")
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
          'GET /state')
            /bin/cp "$fixture_dir/state.json" "$fixture_dir/state-read.json" || exit 1
            /usr/bin/touch "$fixture_dir/state-requested"
            while [ -e "$fixture_dir/state-hold" ]; do /bin/sleep 0.01; done
            /bin/cat "$fixture_dir/state-read.json"
            ;;
          'GET /catalog')
            /usr/bin/touch "$fixture_dir/catalog-requested"
            while [ -e "$fixture_dir/catalog-hold" ]; do /bin/sleep 0.01; done
            /bin/cat "$fixture_dir/catalog.json"
            ;;
          'POST /whatsapp/connect')
            /usr/bin/touch "$fixture_dir/connect-requested"
            while [ -e "$fixture_dir/connect-hold" ]; do /bin/sleep 0.01; done
            /bin/cp "$fixture_dir/connect-response.json" "$fixture_dir/state.json"
            /bin/cat "$fixture_dir/state.json"
            ;;
          'POST /whatsapp/disconnect')
            /usr/bin/touch "$fixture_dir/disconnect-requested"
            while [ -e "$fixture_dir/disconnect-hold" ]; do /bin/sleep 0.01; done
            /bin/cp "$fixture_dir/disconnect-response.json" "$fixture_dir/state.json"
            /bin/cat "$fixture_dir/state.json"
            ;;
          'POST /lifecycle/wake')
            /usr/bin/touch "$fixture_dir/wake-requested"
            while [ -e "$fixture_dir/wake-hold" ]; do /bin/sleep 0.01; done
            /bin/cp "$fixture_dir/wake-response.json" "$fixture_dir/state.json"
            /bin/cat "$fixture_dir/state.json"
            ;;
          'POST /pairing')
            /usr/bin/touch "$fixture_dir/pairing-requested"
            while [ -e "$fixture_dir/pairing-hold" ]; do /bin/sleep 0.01; done
            /bin/cat "$fixture_dir/pairing-response.json"
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
    func remove(_ name: String) { try? FileManager.default.removeItem(at: directory.appendingPathComponent(name)) }
    func scopeRequest() throws -> [String: Any] {
        try XCTUnwrap(JSONSerialization.jsonObject(with: Data(contentsOf: directory.appendingPathComponent("scope-request.json"))) as? [String: Any])
    }
}
