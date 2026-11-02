import Foundation
import LocalAuthentication

/// Confirms that the device owner is present before an action widens agent access.
public protocol UserPresenceVerifying: Sendable {
    /// Returns true only after the device owner authenticates. Cancel or failure returns false.
    func verify(reason: String) async -> Bool
}

/// Touch ID when available, otherwise the login password. Never falls back to allowing.
public struct LocalAuthenticationPresence: UserPresenceVerifying {
    public init() {}
    public func verify(reason: String) async -> Bool {
        let context = LAContext()
        var unavailable: NSError?
        // No passcode, policy unavailable, or any evaluation error: deny.
        guard context.canEvaluatePolicy(.deviceOwnerAuthentication, error: &unavailable) else { return false }
        return (try? await context.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: reason)) ?? false
    }
}
