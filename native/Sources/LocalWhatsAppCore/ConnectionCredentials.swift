import Foundation

/// Credential material returned only by private app control, never normal state.
public struct ConnectionCredentials: Decodable, Sendable {
    public let id: String
    public let name: String
    public let url: String
    public let accessToken: String

    public init?(id: String, name: String, url: String, accessToken: String) {
        guard !id.isEmpty, !name.isEmpty, !accessToken.isEmpty,
              let endpoint = MCPConnectionSummary.localEndpoint(from: url) else { return nil }
        self.id = id
        self.name = name
        self.url = endpoint
        self.accessToken = accessToken
    }

    enum CodingKeys: String, CodingKey {
        case id, name, url
        case accessToken = "access_token"
    }

    public init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        guard let credentials = try Self(
            id: values.decode(String.self, forKey: .id),
            name: values.decode(String.self, forKey: .name),
            url: values.decode(String.self, forKey: .url),
            accessToken: values.decode(String.self, forKey: .accessToken)
        ) else {
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: "Connection details are invalid."))
        }
        self = credentials
    }
}
