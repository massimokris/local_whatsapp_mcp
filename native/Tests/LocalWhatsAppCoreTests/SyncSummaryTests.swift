import XCTest
@testable import LocalWhatsAppCore

final class SyncSummaryTests: XCTestCase {
    func testCountUsesSavedScopeAndVerifiedConversationAliases() throws {
        let catalog = try JSONDecoder().decode(Catalog.self, from: Data(#"{"chats":[{"id":"1@s.whatsapp.net","aliases":["1@s.whatsapp.net","2@lid"]},{"id":"2@lid","aliases":["1@s.whatsapp.net","2@lid"]},{"id":"group@g.us"}]}"#.utf8))
        let selected = Scope(mode: "selected", chatIDs: ["1@s.whatsapp.net", "2@lid"])
        XCTAssertEqual(WhatsAppSyncSummary(whatsapp: .init(), scope: selected, catalog: catalog.chats).connectedChatCount, 1)
        XCTAssertEqual(WhatsAppSyncSummary(whatsapp: .init(), scope: .init(mode: "all"), catalog: catalog.chats).connectedChatCount, 2)
        XCTAssertEqual(WhatsAppSyncSummary(whatsapp: .init(), scope: .init(mode: "selected"), catalog: catalog.chats).connectedChatCount, 0)
        XCTAssertNil(WhatsAppSyncSummary(whatsapp: .init(), scope: selected, catalog: nil).connectedChatCount)
        XCTAssertNil(WhatsAppSyncSummary(whatsapp: .init(), scope: .init(), catalog: catalog.chats).connectedChatCount)
    }

    func testConfirmedCatchUpIgnoresTimestampAge() throws {
        let state = try JSONDecoder().decode(WhatsAppState.self, from: Data(#"{"status":"ready","sync_state":"caught_up","last_event_at":"2026-10-23T12:02:00Z","last_synced_at":"2026-10-23T12:00:00.123Z"}"#.utf8))
        let now = ISO8601DateFormatter().date(from: "2026-10-23T12:02:01Z")!
        let summary = WhatsAppSyncSummary(whatsapp: state, scope: .init(mode: "selected", chatIDs: ["work"]), catalog: [])
        XCTAssertEqual(summary.text(at: now), "Synced just now · 1 chat connected")
        XCTAssertEqual(summary.text(at: now.addingTimeInterval(86400)), "Synced just now · 1 chat connected")
        XCTAssertNotNil(summary.lastSyncedAt)
    }

    func testPendingOrUnknownWorkCannotUseRecentTimestampAsSuccess() throws {
        let now = ISO8601DateFormatter().date(from: "2026-10-23T12:02:01Z")!
        for syncState in ["syncing", "unknown", "future-state"] {
            let state = try JSONDecoder().decode(WhatsAppState.self, from: Data("{\"status\":\"ready\",\"sync_state\":\"\(syncState)\",\"last_synced_at\":\"2026-10-23T12:02:00Z\"}".utf8))
            let summary = WhatsAppSyncSummary(whatsapp: state, scope: .init(mode: "selected", chatIDs: ["work"]), catalog: [])
            XCTAssertEqual(summary.text(at: now), "Syncing messages… · 1 chat connected")
        }
        let unknown = WhatsAppSyncSummary(whatsapp: .init(lastEventAt: "2026-10-23T12:02:00Z"), scope: .init(), catalog: [])
        XCTAssertNil(unknown.lastSyncedAt)
        XCTAssertEqual(unknown.text(at: now), "Syncing messages…")
    }

    func testMissingStateAndDisconnectedTransportNeverShowSuccess() {
        let scope = Scope(mode: "all")
        let catalog = (1...1292).map { Chat(id: "chat-\($0)") }
        let missing = WhatsAppSyncSummary(whatsapp: .init(status: "ready", lastSyncedAt: "2026-10-23T12:00:00Z"), scope: scope, catalog: catalog)
        XCTAssertEqual(missing.text(), "Syncing messages… · 1,292 chats connected")
        let confirmed = WhatsAppSyncSummary(whatsapp: .init(status: "ready", syncState: "caught_up"), scope: scope, catalog: catalog)
        XCTAssertEqual(confirmed.text(), "Synced just now · 1,292 chats connected")
        for status in ["connecting", "disconnected", "failed", "attention"] {
            let disconnected = WhatsAppSyncSummary(whatsapp: .init(status: status, syncState: "caught_up"), scope: scope, catalog: catalog)
            XCTAssertFalse(disconnected.text().contains("Synced just now"))
        }
    }
}
