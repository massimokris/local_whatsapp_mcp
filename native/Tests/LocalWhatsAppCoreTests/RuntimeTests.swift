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

extension RuntimeTests {
    func testOwnerProcessFixture() async throws {
        guard let dir = ProcessInfo.processInfo.environment["LWMCP_OWNER_FIXTURE"] else { return }
        let root = URL(fileURLWithPath: dir), executable = root.appendingPathComponent("helper")
        let runtime = HelperRuntime(helper: executable, wacli: executable, dataDirectory: root)
        try await runtime.start()
        try Data("ready".utf8).write(to: root.appendingPathComponent("owner-ready"))
        try await Task.sleep(nanoseconds: 60_000_000_000)
        await runtime.stop()
    }
    func testAbruptOwnerDeathStopsService() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: dir) }
        let executable = dir.appendingPathComponent("helper")
        try Data("""
        #!/bin/sh
        echo $$ > '\(dir.path)/helper.pid'
        case " $* " in
          *" --owner-stdin "*) exec /bin/cat >/dev/null ;;
          *) exec /bin/sleep 60 ;;
        esac
        """.utf8).write(to: executable)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: executable.path)
        let owner = Process()
        owner.executableURL = URL(fileURLWithPath: "/usr/bin/xcrun")
        owner.arguments = ["xctest", "-XCTest", "LocalWhatsAppCoreTests.RuntimeTests/testOwnerProcessFixture", Bundle(for: RuntimeTests.self).bundlePath]
        owner.environment = ProcessInfo.processInfo.environment.merging(["LWMCP_OWNER_FIXTURE": dir.path]) { _, new in new }
        owner.standardOutput = FileHandle.nullDevice; owner.standardError = FileHandle.nullDevice
        try owner.run()
        var helperPID: Int32 = 0
        defer { if owner.isRunning { owner.terminate() }; if helperPID > 0 { kill(helperPID, SIGKILL) } }
        for _ in 0..<100 {
            if let data = try? String(contentsOf: dir.appendingPathComponent("helper.pid")), let pid = Int32(data.trimmingCharacters(in: .whitespacesAndNewlines)) { helperPID = pid; break }
            try await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertGreaterThan(helperPID, 0)
        guard helperPID > 0 else { return }
        XCTAssertEqual(kill(helperPID, 0), 0)
        kill(owner.processIdentifier, SIGKILL)
        owner.waitUntilExit()
        for _ in 0..<100 where kill(helperPID, 0) == 0 { try await Task.sleep(nanoseconds: 20_000_000) }
        XCTAssertNotEqual(kill(helperPID, 0), 0, "App death must close ownership and stop its service")
    }
}

extension RuntimeTests {
    func testRecordedStartupFailureWinsBeforeServiceExits() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: dir) }
        let executable = dir.appendingPathComponent("helper")
        try Data("""
        #!/bin/sh
        if [ "$1" = "serve" ]; then
          echo '{"startup_error":"configuration_invalid"}' >&2
          exec /bin/sleep 60
        fi
        echo '{"error":"LocalWhatsAppMCP is not running; open the app and retry"}'
        exit 1
        """.utf8).write(to: executable)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: executable.path)
        let runtime = HelperRuntime(helper: executable, wacli: executable, dataDirectory: dir)
        try await runtime.start()
        try await Task.sleep(nanoseconds: 200_000_000)
        let processIdentifier = await runtime.processIdentifier
        XCTAssertNotNil(processIdentifier, "The startup failure must be handled before process exit")
        do {
            _ = try await runtime.control(method: "GET", path: "/state")
            XCTFail("Startup failed")
        } catch {
            XCTAssertTrue(error.localizedDescription.contains("settings.json"), error.localizedDescription)
        }
        await runtime.stop()
    }

    func testStartupFailureKeepsSafeSpecificRecoveryAndBoundsOutput() async throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: dir) }
        let executable = dir.appendingPathComponent("helper")
        try Data("""
        #!/bin/sh
        if [ "$1" = "serve" ]; then
          echo '{"startup_error":"configuration_invalid"}' >&2
          i=0; while [ "$i" -lt 1000 ]; do echo 'private-fixture-token-must-not-be-displayed' >&2; i=$((i+1)); done
          exit 1
        fi
        echo '{"error":"LocalWhatsAppMCP is not running; open the app and retry"}'
        exit 1
        """.utf8).write(to: executable)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: executable.path)
        let runtime = HelperRuntime(helper: executable, wacli: executable, dataDirectory: dir)
        try await runtime.start()
        try await Task.sleep(nanoseconds: 200_000_000)
        do {
            _ = try await runtime.control(method: "GET", path: "/state")
            XCTFail("Startup failed")
        } catch {
            XCTAssertTrue(error.localizedDescription.contains("settings.json"), error.localizedDescription)
            XCTAssertFalse(error.localizedDescription.contains("private-fixture"))
            XCTAssertLessThan(error.localizedDescription.count, 512)
        }
        await runtime.stop()
    }
}

extension RuntimeTests {
    /// Records what the service and control helper receive. `read -r` consumes exactly one line from the pipe.
    private func makeSessionHelper(serveBranch: String = "IFS= read -r t; printf '%s' \"$t\" > \"$dir/serve-token\"; exec /bin/cat >/dev/null") throws -> (URL, HelperRuntime) {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        addTeardownBlock { try? FileManager.default.removeItem(at: dir) }
        let executable = dir.appendingPathComponent("helper")
        try Data("""
        #!/bin/sh
        dir='\(dir.path)'
        if [ "$1" = "serve" ]; then
          printf '%s\\n' "$@" > "$dir/serve-args"
          \(serveBranch)
        fi
        printf '%s\\n' "$@" > "$dir/control-args"
        IFS= read -r t; printf '%s' "$t" > "$dir/control-token"
        /bin/cat > "$dir/control-body"
        printf '{}'
        """.utf8).write(to: executable)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: executable.path)
        let runtime = HelperRuntime(helper: executable, wacli: executable, dataDirectory: dir)
        addTeardownBlock { await runtime.stop() }
        return (dir, runtime)
    }
    private func contents(_ url: URL) async throws -> String {
        for _ in 0..<250 {
            if let value = try? String(contentsOf: url, encoding: .utf8), !value.isEmpty { return value }
            try await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTFail("Timed out waiting for \(url.lastPathComponent)")
        return ""
    }
    private func isSessionToken(_ value: String) -> Bool {
        value.count == 43 && value.allSatisfy { $0.isASCII && ($0.isLetter || $0.isNumber || $0 == "-" || $0 == "_") }
    }

    func testServiceReceivesSessionTokenOnOwnerPipe() async throws {
        let (dir, runtime) = try makeSessionHelper()
        try await runtime.start()
        let token = try await contents(dir.appendingPathComponent("serve-token"))
        XCTAssertTrue(isSessionToken(token), "unexpected token format")
        XCTAssertFalse(try String(contentsOf: dir.appendingPathComponent("serve-args"), encoding: .utf8).contains(token))
    }

    func testControlSendsSameTokenBeforeBody() async throws {
        let (dir, runtime) = try makeSessionHelper()
        try await runtime.start()
        let token = try await contents(dir.appendingPathComponent("serve-token"))
        let body = Data(#"{"mode":"all","chat_ids":[]}"#.utf8)
        let response = try await runtime.control(method: "PUT", path: "/scope", body: body)
        XCTAssertEqual(String(data: response, encoding: .utf8), "{}")
        XCTAssertEqual(try String(contentsOf: dir.appendingPathComponent("control-token"), encoding: .utf8), token)
        XCTAssertEqual(try Data(contentsOf: dir.appendingPathComponent("control-body")), body)
        XCTAssertFalse(try String(contentsOf: dir.appendingPathComponent("control-args"), encoding: .utf8).contains(token))
    }

    func testControlWithoutStartedServiceDoesNotLaunchHelper() async throws {
        let (dir, runtime) = try makeSessionHelper()
        do {
            _ = try await runtime.control(method: "GET", path: "/state")
            XCTFail("Control must not run without a service session")
        } catch {}
        XCTAssertFalse(FileManager.default.fileExists(atPath: dir.appendingPathComponent("control-args").path))
    }

    func testMadeControlTokensAreUniqueAndWellFormed() throws {
        let first = try HelperRuntime.makeControlToken(), second = try HelperRuntime.makeControlToken()
        XCTAssertTrue(isSessionToken(first) && isSessionToken(second))
        XCTAssertNotEqual(first, second)
    }

    /// Slow (about 6 s): a replacement service is only started after the 5 s retry delay.
    func testEachStartUsesFreshToken() async throws {
        let (dir, runtime) = try makeSessionHelper(serveBranch: "IFS= read -r t; printf '%s\\n' \"$t\" >> \"$dir/serve-tokens\"; exit 0")
        let tokens = dir.appendingPathComponent("serve-tokens")
        try await runtime.start()
        _ = try await contents(tokens)
        for _ in 0..<250 where await runtime.processIdentifier != nil { try await Task.sleep(nanoseconds: 20_000_000) }
        try await Task.sleep(nanoseconds: 5_200_000_000)
        try await runtime.start()
        var lines: [String] = []
        for _ in 0..<250 {
            lines = (try? String(contentsOf: tokens, encoding: .utf8))?.split(separator: "\n").map(String.init) ?? []
            if lines.count == 2 { break }
            try await Task.sleep(nanoseconds: 20_000_000)
        }
        XCTAssertEqual(lines.count, 2)
        XCTAssertNotEqual(lines.first, lines.last, "A restarted service must receive a new session token")
    }
}
