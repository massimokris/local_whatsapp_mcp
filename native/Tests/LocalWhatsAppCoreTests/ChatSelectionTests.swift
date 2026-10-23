import XCTest
@testable import LocalWhatsAppCore

final class ChatSelectionTests: XCTestCase {
    func testSearchFindsContactAndFormattedPhoneWithoutChangingDraft() {
        let chats = [Chat(id: "a", name: "Team", phone: "+1 (415) 555-0123", contactName: "Renée"), Chat(id: "b", phone: "44205550123")]
        var selection = ChatSelection(chats: chats, selectedIDs: ["b"])
        selection.query = "415555"
        XCTAssertEqual(selection.results.map(\.id), ["a"])
        selection.query = "renee"
        XCTAssertEqual(selection.results.map(\.id), ["a"])
        selection.chats.append(Chat(id: "new", name: "New conversation"))
        XCTAssertEqual(selection.selectedIDs, ["b"])
    }

    func testSelectAllMatchingSearchIncludesOffscreenChatsAndPreservesOtherSelections() {
        let chats = (1...100).map { Chat(id: "work-\($0)", name: "Work \($0)") }
            + [Chat(id: "family", name: "Family")]
        var selection = ChatSelection(chats: chats, selectedIDs: ["family"])
        selection.query = "work"
        selection.selectAllResults()
        XCTAssertEqual(selection.selectedIDs.count, 101)
        XCTAssertTrue(selection.selectedIDs.contains("work-100"))
        selection.query = "family"
        XCTAssertEqual(selection.results.map(\.id), ["family"])
        selection.query = ""
        XCTAssertEqual(selection.selectedIDs.count, 101)
    }
}

extension ChatSelectionTests {
    func testVerifiedMigrationCanonicalizesDraftAndDeselectsEveryAlias() throws {
        let old = "999@lid", current = "15551234567@s.whatsapp.net"
        let data = Data("""
        [{"id":"\(current)","name":"Work","aliases":["\(current)","\(old)"]},{"id":"777@lid","name":"Work"}]
        """.utf8)
        let migrated = try JSONDecoder().decode([Chat].self, from: data)
        var selection = ChatSelection(chats: [Chat(id: old, name: "Work")], selectedIDs: [old, "777@lid"])
        selection.chats = migrated
        XCTAssertEqual(selection.selectedIDs, [current, "777@lid"])
        selection.toggle(current)
        XCTAssertEqual(selection.selectedIDs, ["777@lid"], "A matching display name is not an identity proof")
        selection.selectedIDs = [old]
        selection.query = "15551234567"
        selection.clearResults()
        XCTAssertTrue(selection.selectedIDs.isEmpty, "Clearing the current row must remove the invisible previous identity")
        selection.selectAllResults()
        XCTAssertEqual(selection.selectedIDs, [current])
    }
}
