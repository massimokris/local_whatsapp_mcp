/// Routing after a fresh service response. Only a foreground open may prepare
/// linking; a background refresh can finish linking without restarting sign-out.
public enum WhatsAppPanelOpening: Equatable {
    case keepCurrentPage
    case home
    case qr(startConnection: Bool)

    public init(state: WhatsAppState, currentPageIsQR: Bool = false, foregroundOpen: Bool = true) {
        if state.disconnectPending == true || state.status == "disconnecting" {
            self = foregroundOpen || currentPageIsQR ? .home : .keepCurrentPage
        } else if state.isLinked {
            // A saved link is enough to skip QR on reopen, even while offline.
            // During an explicit reconnect, keep its error visible until a live
            // connection succeeds or the user reopens the panel.
            self = currentPageIsQR && (foregroundOpen || state.isConnected) ? .home : .keepCurrentPage
        } else if !foregroundOpen {
            self = .keepCurrentPage
        } else {
            let linking = ["connecting", "linking", "logging_in"].contains(state.status ?? "")
            let needsFreshCode = ["attention", "error", "failed"].contains(state.status ?? "") || (state.qr?.isEmpty ?? true)
            self = .qr(startConnection: !linking && needsFreshCode)
        }
    }
}
