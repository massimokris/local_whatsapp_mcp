import XCTest
@testable import LocalWhatsAppCore

final class CloudCallbackTests: XCTestCase {
    func testOnlySingleBoundRequestIsAcceptedForNativeAuthorization() {
        XCTAssertEqual(CloudAuthorizationCallback.request(from: URL(string: "localwhatsappmcp://authorize?request=abcdefghijklmnop_1234567890")!), "abcdefghijklmnop_1234567890")
        for url in ["https://authorize?request=abcdefghijklmnop", "localwhatsappmcp://evil?request=abcdefghijklmnop", "localwhatsappmcp://authorize?request=abcdefghijklmnop&request=second", "localwhatsappmcp://authorize?request=abc&redirect=https://evil.example", "localwhatsappmcp://authorize/path?request=abcdefghijklmnop", "localwhatsappmcp://authorize?request=%0Aecho"] {
            XCTAssertNil(CloudAuthorizationCallback.request(from: URL(string: url)!))
        }
    }
}
