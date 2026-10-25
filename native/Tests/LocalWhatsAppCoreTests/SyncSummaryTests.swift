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

    func testSyncTimestampDecodesIndependentlyOfRecentControlEvents() throws {
        let state = try JSONDecoder().decode(WhatsAppState.self, from: Data(#"{"last_event_at":"2026-10-23T12:02:00Z","last_synced_at":"2026-10-23T12:00:00.123Z"}"#.utf8))
        let now = ISO8601DateFormatter().date(from: "2026-10-23T12:02:01Z")!
        let summary = WhatsAppSyncSummary(whatsapp: state, scope: .init(mode: "selected", chatIDs: ["work"]), catalog: [])
        XCTAssertEqual(summary.text(at: now), "Synced 2 minutes ago - 1 chat connected")
        XCTAssertEqual(summary.text(at: now.addingTimeInterval(-60)), "Synced 1 minute ago - 1 chat connected")
        XCTAssertEqual(summary.text(at: now.addingTimeInterval(-120)), "Synced just now - 1 chat connected")
        let unknown = WhatsAppSyncSummary(whatsapp: .init(lastEventAt: state.lastEventAt), scope: .init(), catalog: [])
        XCTAssertNil(unknown.lastSyncedAt)
        XCTAssertEqual(unknown.text(at: now), "Syncing…")
    }
}
