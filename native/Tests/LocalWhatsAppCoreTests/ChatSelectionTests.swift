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
