import Foundation

// MARK: - Model  (mirrors models/model.rs)

public struct AIModel: Codable, Sendable, Hashable, Identifiable {
    public let id: String
    public let provider: String
    public let contextWindow: UInt64

    enum CodingKeys: String, CodingKey {
        case id
        case provider
        case contextWindow = "contextWindow"
    }
}

// MARK: - ProviderCatalogEntry

public struct ProviderCatalogEntry: Codable, Sendable, Hashable, Identifiable {
    public var id: String { name }
    public let name: String
    public let displayName: String
    public let description: String
    public let models: [AIModel]

    enum CodingKeys: String, CodingKey {
        case name
        case displayName  = "displayName"
        case description
        case models
    }
}
