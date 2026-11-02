import XCTest
@testable import LocalWhatsAppCore

final class UserPresenceTests: XCTestCase {
    /// Never evaluates a real LAContext: automated runs must not show system prompts.
    func testLocalAuthenticationPresenceIsASendableVerifier() {
        func requireSendable<T: Sendable>(_ value: T) -> T { value }
        let presence: any UserPresenceVerifying = requireSendable(LocalAuthenticationPresence())
        XCTAssertTrue(presence is LocalAuthenticationPresence)
    }
}
