import XCTest
import LocalWhatsAppCore

final class ServiceStateTests: XCTestCase {
    func testWhatsAppStateIgnoresRemovedCompletionMetadata() throws {
        let data = Data(#"{"status":"ready","linked":true,"sync_state":"error","last_synced_at":"2026-10-01T12:00:00Z"}"#.utf8)
        let state = try JSONDecoder().decode(WhatsAppState.self, from: data)
        XCTAssertTrue(state.isConnected)
        let encoded = try XCTUnwrap(JSONSerialization.jsonObject(with: JSONEncoder().encode(state)) as? [String: Any])
        XCTAssertNil(encoded["sync_state"])
        XCTAssertNil(encoded["last_synced_at"])
        XCTAssertEqual(encoded["status"] as? String, "ready")
    }

    func testLegacyProgressStatusRequiresLinkedAccount() {
        for status in ["syncing", "importing", "receiving_history"] {
            XCTAssertFalse(WhatsAppState(status: status, linked: false).isConnected)
            XCTAssertFalse(WhatsAppState(status: status).isLinked)
            XCTAssertTrue(WhatsAppState(status: status, linked: true).isConnected)
        }
        for status in ["attention", "error", "failed", "connecting", "disconnected"] {
            XCTAssertFalse(WhatsAppState(status: status, linked: true).isConnected)
        }
    }
}
