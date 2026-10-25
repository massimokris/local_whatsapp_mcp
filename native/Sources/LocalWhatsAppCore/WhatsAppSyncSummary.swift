import Foundation

/// Freshness comes from a completed sync; connected chats come from saved scope,
/// independently of search filters and unsaved selections in the chooser.
public struct WhatsAppSyncSummary {
    public let lastSyncedAt: Date?
    public let connectedChatCount: Int?

    public init(whatsapp: WhatsAppState, scope: Scope, catalog: [Chat]?) {
        let formatter = ISO8601DateFormatter()
        let timestamp = whatsapp.lastSyncedAt ?? ""
        if let date = formatter.date(from: timestamp) {
            lastSyncedAt = date
        } else {
            formatter.formatOptions.insert(.withFractionalSeconds)
            lastSyncedAt = formatter.date(from: timestamp)
        }
        if let catalog, scope.isConfirmed {
            let saved = ChatSelection(chats: catalog, selectedIDs: Set(scope.chatIDs))
            connectedChatCount = scope.mode == "all" ? saved.results.count : saved.selectedIDs.count
        } else {
            connectedChatCount = nil
        }
    }

    public func text(at now: Date = Date()) -> String {
        let freshness: String
        if let lastSyncedAt {
            let minutes = Int(max(0, now.timeIntervalSince(lastSyncedAt)) / 60)
            freshness = minutes == 0 ? "Synced just now" : "Synced \(minutes) \(minutes == 1 ? "minute" : "minutes") ago"
        } else {
            freshness = "Syncing…"
        }
        guard let connectedChatCount else { return freshness }
        return "\(freshness) - \(connectedChatCount) \(connectedChatCount == 1 ? "chat" : "chats") connected"
    }
}
