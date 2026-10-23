import XCTest
@testable import LocalWhatsAppCore

final class StateTests: XCTestCase {
    func testMissingOptionalStatusFieldsPreserveInitialAccessBoundary() throws {
        let state = try JSONDecoder().decode(ServiceState.self, from: Data(#"{"whatsapp":{"status":"linking"},"scope":{"mode":"initial"}}"#.utf8))
        XCTAssertEqual(state.scope.mode, "initial")
        XCTAssertFalse(state.scope.isConfirmed)
        XCTAssertEqual(state.agents.count, 0)
        XCTAssertNil(state.whatsapp.qr)
    }
}
