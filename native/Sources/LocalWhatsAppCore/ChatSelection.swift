import Foundation

public struct Chat: Codable, Identifiable, Equatable, Sendable {
    public var id: String
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
        case id, name, phone, kind
        case contactName = "contact_name", lastActivity = "last_activity"
    }
}

/// A draft belongs to the panel session, so filtering and incidental dismissal never change scope.
public struct ChatSelection: Sendable {
    public var chats: [Chat]
    public var selectedIDs: Set<String>
    public var query = ""
    public init(chats: [Chat] = [], selectedIDs: Set<String> = []) {
        self.chats = chats; self.selectedIDs = selectedIDs
    }
    public var results: [Chat] {
        chats.filter { chat in
            let term = query.trimmingCharacters(in: .whitespacesAndNewlines)
            let digits = term.filter(\.isNumber)
            let isPhoneQuery = !digits.isEmpty && term.allSatisfy { $0.isNumber || "+()- .".contains($0) }
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
