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
    private var owner: FileHandle?
    private var diagnostics: StartupDiagnostics?
    private var diagnosticPipe: Pipe?
    private var retryAfter = Date.distantPast
    private var stopping = false
    public init(helper: URL, wacli: URL, dataDirectory: URL) {
        self.helper = helper; self.wacli = wacli; self.dataDirectory = dataDirectory
    }
    public var processIdentifier: Int32? { service?.isRunning == true ? service?.processIdentifier : nil }
    public func start() throws {
        guard !stopping, service?.isRunning != true else { return }
        if service != nil, Date() < retryAfter {
            throw diagnostics?.failure ?? HelperFailure("The local service stopped. Retrying shortly; reopen the app if this continues.")
        }
        try? owner?.close(); owner = nil
        diagnosticPipe?.fileHandleForReading.readabilityHandler = nil
        try? diagnosticPipe?.fileHandleForReading.close()
        guard FileManager.default.isExecutableFile(atPath: helper.path), FileManager.default.isExecutableFile(atPath: wacli.path) else {
            throw HelperFailure("The bundled service is missing. Reinstall the complete LocalWhatsAppMCP app.")
        }
        let process = Process()
        process.executableURL = helper
        process.arguments = ["serve", "--owner-stdin", "--data-dir", dataDirectory.path, "--wacli", wacli.path]
        process.standardOutput = FileHandle.nullDevice
        let capture = StartupDiagnostics(), errors = Pipe()
        errors.fileHandleForReading.readabilityHandler = { handle in
            let data = handle.availableData
            if data.isEmpty { handle.readabilityHandler = nil }
            else { capture.append(data) }
        }
        process.standardError = errors
        diagnostics = capture; diagnosticPipe = errors
        let lifetime = Pipe()
        process.standardInput = lifetime.fileHandleForReading
        try process.run()
        try errors.fileHandleForWriting.close()
        try lifetime.fileHandleForReading.close()
        owner = lifetime.fileHandleForWriting
        service = process
        retryAfter = Date().addingTimeInterval(5)
    }
    public func stop() async {
        stopping = true
        try? owner?.close()
        owner = nil
        guard let process = service else { return }
        if process.isRunning {
            process.terminate()
            // The service waits for its engine. Keep the UI responsive while it does so.
            for _ in 0..<200 where process.isRunning { try? await Task.sleep(nanoseconds: 100_000_000) }
            if process.isRunning { kill(process.processIdentifier, SIGKILL) }
            // An actor can resume on another thread after suspension. Foundation's
            // run-loop-based waitUntilExit can then stall even after the child exits.
            while process.isRunning { try? await Task.sleep(nanoseconds: 100_000_000) }
        }
        service = nil
        diagnosticPipe?.fileHandleForReading.readabilityHandler = nil
        try? diagnosticPipe?.fileHandleForReading.close()
        diagnosticPipe = nil
    }
    public func control(method: String, path: String, body: Data? = nil) async throws -> Data {
        guard !stopping else { throw HelperFailure("The local service is stopping.") }
        // A startup_error is terminal even while the process is still draining
        // stderr or shutting down. Its safe recovery message must not be replaced
        // by the control helper's generic connection error during that interval.
        if let failure = diagnostics?.failure { throw failure }
        if service?.isRunning == false {
            throw HelperFailure("The local service stopped. Reopen the app or wait for it to retry.")
        }
        do {
            return try await Self.run(helper: helper, arguments: ["control", "--data-dir", dataDirectory.path, "--method", method, "--path", path], input: body)
        } catch {
            if let failure = diagnostics?.failure { throw failure }
            throw error
        }
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

/// Drains stderr without keeping logs. Only a fixed startup code becomes UI copy.
private final class StartupDiagnostics: @unchecked Sendable {
    private let lock = NSLock()
    private var prefix = Data()
    func append(_ data: Data) {
        lock.lock(); defer { lock.unlock() }
        prefix.append(data.prefix(max(0, 8192 - prefix.count)))
    }
    var failure: HelperFailure? {
        lock.lock(); let bytes = prefix; lock.unlock()
        for line in bytes.split(separator: 10) {
            guard let value = try? JSONSerialization.jsonObject(with: Data(line)) as? [String: String],
                  let code = value["startup_error"], let message = Self.messages[code] else { continue }
            return HelperFailure(message)
        }
        return nil
    }
    private static let messages = [
        "configuration_missing": "Saved settings.json is missing. Restore it in the app’s Application Support folder before reopening this archive.",
        "configuration_invalid": "Saved settings.json is invalid. Restore a valid copy in the app’s Application Support folder and retry.",
        "credentials_unavailable": "The app’s credentials.db is missing or unavailable. Restore it in the app’s Application Support folder and reopen the app.",
        "port_in_use": "The local MCP port is in use. Quit the other LocalWhatsAppMCP service and retry.",
        "already_running": "Another LocalWhatsAppMCP service is still running. Quit that app instance and retry.",
        "writer_stopping": "The previous WhatsApp connection is still stopping. Wait briefly, then reopen the app.",
        "control_unavailable": "The private app connection could not start. Check access to the app’s Application Support folder and reopen the app.",
        "storage_unavailable": "The app could not access its saved files. Check permissions and free space in its Application Support folder, then retry.",
        "startup_failed": "The bundled service could not start. Reopen the app; reinstall the complete app if this continues."
    ]
}
