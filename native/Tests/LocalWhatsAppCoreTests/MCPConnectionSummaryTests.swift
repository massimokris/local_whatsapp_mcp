import XCTest
@testable import LocalWhatsAppCore

final class MCPConnectionSummaryTests: XCTestCase {
    private func state(scope: Scope = .init(mode: "all"), status: String? = "ready", url: String? = "http://127.0.0.1:32191") -> ServiceState {
        ServiceState(whatsapp: .init(status: "disconnected", linked: false), scope: scope, mcp: .init(url: url, status: status))
    }

    func testUnavailableServiceCannotReuseStaleReadyState() {
        let summary = MCPConnectionSummary(state: state(), serviceAvailable: false)
        XCTAssertEqual(summary.statusText, "Local server unavailable")
        XCTAssertFalse(summary.isReady)
        XCTAssertFalse(summary.canPair)
    }

    func testConfirmedScopeAllowsArchiveAccessWhileWhatsAppIsDisconnected() {
        for scope in [Scope(mode: "all"), Scope(mode: "selected") ] {
            let summary = MCPConnectionSummary(state: state(scope: scope), serviceAvailable: true)
            XCTAssertTrue(summary.isReady)
            XCTAssertTrue(summary.canPair)
            XCTAssertEqual(summary.statusText, "Ready on this Mac")
        }
        let initial = MCPConnectionSummary(state: state(scope: .init()), serviceAvailable: true)
        XCTAssertFalse(initial.isReady)
        XCTAssertFalse(initial.canPair)
        XCTAssertEqual(initial.statusText, "Choose chats to enable MCP")
    }

    func testTransitionAndUnknownStatusesCannotAdvertiseReadiness() {
        for status in ["setup_required", "applying_scope", "unknown", nil] {
            let summary = MCPConnectionSummary(state: state(status: status), serviceAvailable: true)
            XCTAssertFalse(summary.isReady)
            XCTAssertFalse(summary.canPair)
        }
        XCTAssertEqual(MCPConnectionSummary(state: state(status: "applying_scope"), serviceAvailable: true).statusText, "Updating chat access…")
        XCTAssertEqual(MCPConnectionSummary(state: state(status: "setup_required"), serviceAvailable: true).statusText, "Choose chats to enable MCP")
    }

    func testEndpointAppendsMCPExactlyOnce() {
        for suffix in ["", "/", "/mcp", "/mcp/"] {
            let summary = MCPConnectionSummary(state: state(url: "http://127.0.0.1:32191" + suffix), serviceAvailable: true)
            XCTAssertEqual(summary.endpoint, "http://127.0.0.1:32191/mcp")
        }
        for host in ["localhost", "127.2.3.4", "[::1]", "[0:0:0:0:0:0:0:1]", "[::ffff:127.0.0.1]"] {
            XCTAssertTrue(MCPConnectionSummary(state: state(url: "http://\(host):32191"), serviceAvailable: true).isReady, host)
        }
    }

    func testInvalidOrExternalEndpointCannotEnablePairing() {
        for url in [nil, "", "127.0.0.1:32191", "https://127.0.0.1:32191", "http://example.com", "http://0.0.0.0:32191", "http://192.168.1.2:32191", "http://[::]:32191", "http://[::ffff:192.168.1.2]:32191", "http://127.0.0.1.example.com", "http://127.0.0.1:99999", "http://127.0.0.1:0", "http://user:secret@127.0.0.1", "http://127.0.0.1/?token=fixture", "http://127.0.0.1/#fixture", "http://127.0.0.1/unrelated"] {
            let summary = MCPConnectionSummary(state: state(url: url), serviceAvailable: true)
            XCTAssertNil(summary.endpoint, url ?? "nil")
            XCTAssertFalse(summary.isReady)
            XCTAssertFalse(summary.canPair)
        }
    }
}
