import Foundation

public enum CloudAuthorizationCallback {
    public static func request(from url: URL) -> String? {
        guard let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              components.scheme == "localwhatsappmcp", components.host == "authorize",
              components.path.isEmpty, components.user == nil, components.password == nil,
              components.port == nil, components.fragment == nil,
              let items = components.queryItems, items.count == 1, items[0].name == "request",
              let request = items[0].value, (16...256).contains(request.utf8.count),
              request.utf8.allSatisfy({ (65...90).contains($0) || (97...122).contains($0) || (48...57).contains($0) || $0 == 45 || $0 == 95 }) else { return nil }
        return request
    }
}
