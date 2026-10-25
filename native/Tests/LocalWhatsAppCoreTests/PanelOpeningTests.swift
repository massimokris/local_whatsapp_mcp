import XCTest
@testable import LocalWhatsAppCore

final class PanelOpeningTests: XCTestCase {
    func testUnlinkedAccountOpensQRAndStartsLinking() {
        XCTAssertEqual(WhatsAppPanelOpening(state: .init(status: "disconnected", linked: false)), .qr(startConnection: true))
        XCTAssertEqual(WhatsAppPanelOpening(state: .init()), .qr(startConnection: true))
    }

    func testExistingQRCodeOrInFlightLinkDoesNotStartAgain() {
        for state in [WhatsAppState(status: "connecting", linked: false),
                      WhatsAppState(status: "linking", linked: false),
                      WhatsAppState(qr: "synthetic-pairing-code", linked: false)] {
            XCTAssertEqual(WhatsAppPanelOpening(state: state), .qr(startConnection: false))
        }
        XCTAssertEqual(WhatsAppPanelOpening(state: .init(status: "attention", qr: "expired-code", linked: false)), .qr(startConnection: true))
    }

    func testLinkedAccountsKeepTheirPageEvenWhileOffline() {
        for status in ["connected", "syncing", "attention", "failed", "disconnected", "connecting"] {
            XCTAssertEqual(WhatsAppPanelOpening(state: .init(status: status, linked: true)), .keepCurrentPage)
        }
        // Older/partial state responses can still prove a live link.
        XCTAssertEqual(WhatsAppPanelOpening(state: .init(status: "receiving_history")), .keepCurrentPage)
    }

    func testFirstSuccessfulLinkAutomaticallyLeavesQRCodeForHome() {
        for status in ["connected", "ready", "syncing", "importing", "receiving_history"] {
            XCTAssertEqual(WhatsAppPanelOpening(state: .init(status: status, linked: true), currentPageIsQR: true, foregroundOpen: false), .home)
        }
    }

    func testReopeningSavedLinkedAccountLeavesStaleQRCodeEvenWhenOffline() {
        for status in ["connected", "syncing", "attention", "failed", "disconnected", "connecting"] {
            XCTAssertEqual(WhatsAppPanelOpening(state: .init(status: status, linked: true), currentPageIsQR: true), .home)
        }
    }

    func testBackgroundRefreshKeepsExplicitQRCodeReconnectFailureVisible() {
        for linked in [true, false] {
            XCTAssertEqual(WhatsAppPanelOpening(state: .init(status: "attention", linked: linked), currentPageIsQR: true, foregroundOpen: false), .keepCurrentPage)
        }
    }

    func testBackgroundUnlinkedAccountDoesNotLaunchLinkingAfterIntentionalDisconnect() {
        XCTAssertEqual(WhatsAppPanelOpening(state: .init(status: "disconnected", linked: false), foregroundOpen: false), .keepCurrentPage)
    }

    func testLinkedAccountDoesNotInterruptChatOrAgentManagementDuringRefresh() {
        for foregroundOpen in [true, false] {
            for status in ["ready", "syncing", "connecting", "attention"] {
                XCTAssertEqual(WhatsAppPanelOpening(state: .init(status: status, linked: true), currentPageIsQR: false, foregroundOpen: foregroundOpen), .keepCurrentPage)
            }
        }
    }

    func testLeavingQRCodeDoesNotConfirmInitialChatScope() {
        let state = ServiceState(whatsapp: .init(status: "ready", linked: true))
        XCTAssertEqual(WhatsAppPanelOpening(state: state.whatsapp, currentPageIsQR: true, foregroundOpen: false), .home)
        XCTAssertFalse(state.scope.isConfirmed)
        XCTAssertFalse(MCPConnectionSummary(state: state, serviceAvailable: true).canPair)
    }

    func testUnfinishedDisconnectAlwaysReturnsToRetryHome() {
        for linked in [true, false] {
            XCTAssertEqual(WhatsAppPanelOpening(state: .init(status: "attention", linked: linked, disconnectPending: true)), .home)
            XCTAssertEqual(WhatsAppPanelOpening(state: .init(status: "disconnecting", linked: linked)), .home)
        }
    }
}
