import XCTest
@testable import LocalWhatsAppCore

final class RuntimeTests: XCTestCase {
    func testShutdownRejectsNewControlRequests() async throws {
        let executable = URL(fileURLWithPath: "/usr/bin/true")
        let runtime = HelperRuntime(helper: executable, wacli: executable, dataDirectory: FileManager.default.temporaryDirectory)
        await runtime.stop()
        do {
            _ = try await runtime.control(method: "PUT", path: "/scope", body: Data("{}".utf8))
            XCTFail("A new request must not begin during shutdown")
        } catch { XCTAssertTrue(error.localizedDescription.contains("stopping")) }
    }

    func testLifecycleStartsOneOwnedServiceAndStopsIt() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: dir) }
        let executable = dir.appendingPathComponent("helper")
        try Data("#!/bin/sh\nexec /bin/sleep 60\n".utf8).write(to: executable)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: executable.path)
        let runtime = HelperRuntime(helper: executable, wacli: executable, dataDirectory: dir)
        try await runtime.start()
        let first = await runtime.processIdentifier
        try await runtime.start()
        let second = await runtime.processIdentifier
        XCTAssertNotNil(first)
        XCTAssertEqual(first, second)
        await runtime.stop()
        let stopped = await runtime.processIdentifier
        XCTAssertNil(stopped)
    }
}
