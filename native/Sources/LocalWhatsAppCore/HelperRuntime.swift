import Foundation
import Darwin

public struct HelperFailure: LocalizedError, Sendable {
    public let message: String
    public var errorDescription: String? { message }
    public init(_ message: String) { self.message = message }
}

/// Serializes app-owned service lifecycle. All process and file I/O runs away from the main actor.
public actor HelperRuntime {
    public let helper: URL
    public let wacli: URL
    public let dataDirectory: URL
    private var service: Process?
    private var stopping = false
    public init(helper: URL, wacli: URL, dataDirectory: URL) {
        self.helper = helper; self.wacli = wacli; self.dataDirectory = dataDirectory
    }
    public var processIdentifier: Int32? { service?.isRunning == true ? service?.processIdentifier : nil }
    public func start() throws {
        guard !stopping, service?.isRunning != true else { return }
        guard FileManager.default.isExecutableFile(atPath: helper.path), FileManager.default.isExecutableFile(atPath: wacli.path) else {
            throw HelperFailure("The bundled service is missing. Reinstall the complete LocalWhatsAppMCP app.")
        }
        let process = Process()
        process.executableURL = helper
        process.arguments = ["serve", "--data-dir", dataDirectory.path, "--wacli", wacli.path, "--helper", helper.path]
        process.standardOutput = FileHandle.nullDevice
        process.standardError = FileHandle.nullDevice
        process.standardInput = FileHandle.nullDevice
        try process.run()
        service = process
    }
    public func stop() async {
        stopping = true
        guard let process = service else { return }
        if process.isRunning {
            process.terminate()
            // The service waits for its engine and tunnel. Keep the UI responsive while it does so.
            for _ in 0..<200 where process.isRunning { try? await Task.sleep(nanoseconds: 100_000_000) }
            if process.isRunning { kill(process.processIdentifier, SIGKILL) }
            process.waitUntilExit()
        }
        service = nil
    }
    public func control(method: String, path: String, body: Data? = nil) async throws -> Data {
        guard !stopping else { throw HelperFailure("The local service is stopping.") }
        return try await Self.run(helper: helper, arguments: ["control", "--data-dir", dataDirectory.path, "--method", method, "--path", path], input: body)
    }
    public func authorizeCloud(request: String) async throws {
        guard !stopping else { throw HelperFailure("The local service is stopping.") }
        _ = try await Self.run(helper: helper, arguments: ["cloud-authorize", "--data-dir", dataDirectory.path, "--request", request], input: nil)
    }
    private static func run(helper: URL, arguments: [String], input: Data?) async throws -> Data {
        try await Task.detached(priority: .userInitiated) {
            let process = Process(), output = Pipe(), stdin = Pipe()
            process.executableURL = helper; process.arguments = arguments
            process.standardOutput = output; process.standardError = FileHandle.nullDevice
            process.standardInput = stdin
            try process.run()
            let timeout = DispatchWorkItem { if process.isRunning { process.terminate() } }
            DispatchQueue.global(qos: .utility).asyncAfter(deadline: .now() + 45, execute: timeout)
            defer { timeout.cancel() }
            if let input { try stdin.fileHandleForWriting.write(contentsOf: input) }
            try stdin.fileHandleForWriting.close()
            let result = output.fileHandleForReading.readDataToEndOfFile()
            process.waitUntilExit()
            if let error = (try? JSONSerialization.jsonObject(with: result)) as? [String: Any], let message = error["error"] as? String, !message.isEmpty {
                throw HelperFailure(message)
            }
            guard process.terminationStatus == 0 else {
                throw HelperFailure("The local service could not complete this request. Retry after checking its status.")
            }
            return result
        }.value
    }
}
