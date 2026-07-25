import Foundation

// MARK: - SessionHeader  (mirrors models/session.rs)

public struct SessionHeader: Codable, Sendable, Hashable, Identifiable {
    public let id: String
    public var title: String
    public let cwd: String
    public let projectId: String?
    public let modelId: String
    public let provider: String
    public let createdAt: UInt64
    public let updatedAt: UInt64
    public let messageCount: UInt64?

    enum CodingKeys: String, CodingKey {
        case id
        case title
        case cwd
        case projectId   = "projectId"
        case modelId     = "modelId"
        case provider
        case createdAt   = "createdAt"
        case updatedAt   = "updatedAt"
        case messageCount = "messageCount"
    }
}

// MARK: - SessionDetailResponse

public struct SessionDetailResponse: Codable, Sendable, Hashable {
    public let header: SessionHeader
    public let messages: [AgentMessage]
}
