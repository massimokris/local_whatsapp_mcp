import Foundation

public struct Chat: Codable, Identifiable, Equatable, Sendable {
    public var id: String
    public var aliases: [String]?
    public var name: String?
    public var phone: String?
    public var contactName: String?
    public var kind: String?
    public var lastActivity: String?

    public init(id: String, name: String? = nil, phone: String? = nil, contactName: String? = nil, kind: String? = nil, lastActivity: String? = nil) {
        self.id = id; self.name = name; self.phone = phone; self.contactName = contactName
        self.kind = kind; self.lastActivity = lastActivity
    }
    public var displayName: String {
        [name, contactName, phone, id].compactMap { $0 }.first { !$0.trimmingCharacters(in: .whitespaces).isEmpty } ?? id
    }
    enum CodingKeys: String, CodingKey {
        case id, name, phone, kind, aliases
        case contactName = "contact_name", lastActivity = "last_activity"
    }
}

/// A draft belongs to the panel session, so filtering and incidental dismissal never change scope.
public struct ChatSelection: Sendable {
    public var chats: [Chat] {
        didSet { selectedIDs = canonicalSelection(selectedIDs) }
    }
    public var selectedIDs: Set<String> {
        didSet { selectedIDs = canonicalSelection(selectedIDs) }
    }
    public var query = ""
    public init(chats: [Chat] = [], selectedIDs: Set<String> = []) {
        self.chats = chats; self.selectedIDs = selectedIDs
        self.selectedIDs = canonicalSelection(selectedIDs)
    }
    // Only the server's verified identity groups may merge rows or selections.
    private var canonicalIDs: [String: String] {
        var mapping: [String: String] = [:]
        let ordered = chats.sorted {
            let leftPhone = $0.id.hasSuffix("@s.whatsapp.net"), rightPhone = $1.id.hasSuffix("@s.whatsapp.net")
            return leftPhone == rightPhone ? $0.id < $1.id : leftPhone
        }
        for chat in ordered {
            let aliases = [chat.id] + (chat.aliases ?? [])
            let canonical = aliases.compactMap { mapping[$0] }.first ?? chat.id
            for alias in aliases { mapping[alias] = canonical }
        }
        return mapping
    }
    private func canonicalSelection(_ ids: Set<String>) -> Set<String> {
        let mapping = canonicalIDs
        return Set(ids.map { mapping[$0] ?? $0 })
    }
    public var results: [Chat] {
        let mapping = canonicalIDs
        let term = query.trimmingCharacters(in: .whitespacesAndNewlines)
        let digits = term.filter(\.isNumber)
        let isPhoneQuery = !digits.isEmpty && term.allSatisfy { $0.isNumber || "+()- .".contains($0) }
        return chats.filter { chat in
            guard mapping[chat.id] == chat.id else { return false }
            return term.isEmpty || [chat.displayName, chat.contactName ?? "", chat.phone ?? "", chat.id]
                .contains { $0.range(of: term, options: [.caseInsensitive, .diacriticInsensitive]) != nil }
                || (isPhoneQuery && (chat.phone ?? "").filter(\.isNumber).contains(digits))
        }.sorted {
            if $0.lastActivity != $1.lastActivity { return ($0.lastActivity ?? "") > ($1.lastActivity ?? "") }
            return $0.displayName.localizedStandardCompare($1.displayName) == .orderedAscending
        }
    }
    public mutating func selectAllResults() { selectedIDs.formUnion(results.map(\.id)) }
    public mutating func clearResults() { selectedIDs.subtract(results.map(\.id)) }
    public mutating func toggle(_ id: String) {
        if selectedIDs.contains(id) { selectedIDs.remove(id) } else { selectedIDs.insert(id) }
    }
}
