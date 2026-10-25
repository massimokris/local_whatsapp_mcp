import Foundation

/// Saved grants and actual request activity are separate: an unused credential
/// must not look connected merely because the local service is running.
public struct AgentConnectionSummary: Sendable {
    private let agent: AgentConnection

    public init(agent: AgentConnection) { self.agent = agent }

    public func text(at now: Date = Date()) -> String {
        let sending = agent.sendEnabled == true ? "Message sending on" : "Message sending off"
        guard let date = parseServiceTimestamp(agent.lastSeenAt) else {
            return "No request yet - \(sending)"
        }
        let minutes = Int(max(0, now.timeIntervalSince(date)) / 60)
        let activity: String
        if minutes < 1 { activity = "Last request just now" }
        else if minutes < 60 { activity = "Last request \(minutes)m ago" }
        else if minutes < 1440 { activity = "Last request \(minutes / 60)h ago" }
        else { activity = "Last request \(minutes / 1440)d ago" }
        return "\(activity) - \(sending)"
    }

    public var lastUsedText: String {
        guard let date = parseServiceTimestamp(agent.lastSeenAt) else { return "No request yet" }
        let formatter = DateFormatter()
        formatter.dateStyle = .short
        formatter.timeStyle = .short
        return "Last used at \(formatter.string(from: date))"
    }

    public static func nextName(in agents: [AgentConnection]) -> String {
        let existing = Set(agents.map { $0.name.lowercased() })
        var number = 1
        while existing.contains("connection \(number)") { number += 1 }
        return "Connection \(number)"
    }
}
