import Foundation

/// Catch-up comes from explicit current completion; connected chats come from saved scope,
/// independently of search filters and unsaved selections in the chooser.
public struct WhatsAppSyncSummary {
    public let lastSyncedAt: Date?
    public let connectedChatCount: Int?
    private let isCaughtUp: Bool

    public init(whatsapp: WhatsAppState, scope: Scope, catalog: [Chat]?) {
        lastSyncedAt = parseServiceTimestamp(whatsapp.lastSyncedAt)
        isCaughtUp = whatsapp.isConnected && whatsapp.syncState == "caught_up"
        if let catalog, scope.isConfirmed {
            let saved = ChatSelection(chats: catalog, selectedIDs: Set(scope.chatIDs))
            connectedChatCount = scope.mode == "all" ? saved.results.count : saved.selectedIDs.count
        } else {
            connectedChatCount = nil
        }
    }

    public func text(at _: Date = Date()) -> String {
        let freshness = isCaughtUp ? "Synced just now" : "Syncing messages…"
        guard let connectedChatCount else { return freshness }
        return "\(freshness) · \(connectedChatCount.formatted(.number.locale(Locale(identifier: "en_US")))) \(connectedChatCount == 1 ? "chat" : "chats") connected"
    }
}
