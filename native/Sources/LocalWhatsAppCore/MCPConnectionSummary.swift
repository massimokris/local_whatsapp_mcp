import Foundation
import Darwin

/// MCP archive access depends on the local service and saved scope, independently
/// of whether WhatsApp is currently online.
public struct MCPConnectionSummary: Sendable {
    public let endpoint: String?
    public let statusText: String
    public let isReady: Bool

    public init(state: ServiceState, serviceAvailable: Bool) {
        endpoint = Self.localEndpoint(from: state.mcp.url)
        isReady = serviceAvailable && state.scope.isConfirmed && state.mcp.status == "ready" && endpoint != nil

        if !serviceAvailable {
            statusText = "Local server unavailable"
        } else if !state.scope.isConfirmed || state.mcp.status == "setup_required" {
            statusText = "Choose chats to enable MCP"
        } else if state.mcp.status == "applying_scope" {
            statusText = "Updating chat access…"
        } else if isReady {
            statusText = "Ready on this Mac"
        } else {
            statusText = "Local server not ready"
        }
    }

    static func localEndpoint(from raw: String?) -> String? {
        guard let raw, var components = URLComponents(string: raw),
              components.scheme?.lowercased() == "http",
              let host = components.host, isLoopback(host),
              components.user == nil, components.password == nil,
              components.query == nil, components.fragment == nil,
              components.port.map({ (1...65535).contains($0) }) ?? true,
              ["", "/", "/mcp", "/mcp/"].contains(components.path) else { return nil }
        components.path = "/mcp"
        return components.url?.absoluteString
    }

    private static func isLoopback(_ host: String) -> Bool {
        if host.lowercased() == "localhost" { return true }
        let address = host.hasPrefix("[") && host.hasSuffix("]") ? String(host.dropFirst().dropLast()) : host
        var ipv4 = in_addr()
        if inet_pton(AF_INET, address, &ipv4) == 1 {
            return withUnsafeBytes(of: ipv4) { $0[0] == 127 }
        }
        var ipv6 = in6_addr()
        guard inet_pton(AF_INET6, address, &ipv6) == 1 else { return false }
        return withUnsafeBytes(of: ipv6) { bytes in
            let loopback = bytes.prefix(15).allSatisfy { $0 == 0 } && bytes[15] == 1
            let mappedLoopback = bytes.prefix(10).allSatisfy { $0 == 0 }
                && bytes[10] == 255 && bytes[11] == 255 && bytes[12] == 127
            return loopback || mappedLoopback
        }
    }
}
