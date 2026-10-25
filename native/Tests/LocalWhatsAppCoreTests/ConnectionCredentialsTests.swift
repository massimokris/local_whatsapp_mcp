import XCTest
@testable import LocalWhatsAppCore

final class ConnectionCredentialsTests: XCTestCase {
    func testPrivateResponseKeepsAccessTokenAndNormalizesLocalEndpoint() throws {
        let data = Data(#"{"id":"synthetic-agent","name":"Manual client","url":"http://127.0.0.1:32191","access_token":"synthetic-access-token"}"#.utf8)
        let credentials = try JSONDecoder().decode(ConnectionCredentials.self, from: data)
        XCTAssertEqual(credentials.url, "http://127.0.0.1:32191/mcp")
        XCTAssertEqual(credentials.accessToken, "synthetic-access-token")
    }

    func testCredentialCannotBePresentedWithExternalEndpointOrEmptyToken() throws {
        for (url, token) in [("https://example.com/mcp", "synthetic-token"), ("http://127.0.0.1:32191/mcp", "")] {
            let data = try JSONSerialization.data(withJSONObject: ["id": "synthetic-agent", "name": "Manual client", "url": url, "access_token": token])
            XCTAssertThrowsError(try JSONDecoder().decode(ConnectionCredentials.self, from: data))
        }
    }
}
