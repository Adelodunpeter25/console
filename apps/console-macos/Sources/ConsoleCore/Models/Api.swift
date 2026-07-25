import Foundation

// MARK: - ApiResponse<T>  (mirrors models/api.rs)
// Decodable-only: we never encode this — we only decode server responses.
public struct ApiResponse<T: Decodable & Sendable>: Decodable {
    public let success: Bool
    public let data: T?
    public let error: String?
}

// MARK: - Request DTOs (mirror packages/types/src/api.ts)

public struct CreateSessionDto: Codable, Sendable {
    public let cwd: String
    public let projectId: String?
    public let modelId: String?
    public let provider: String?
    public let title: String?

    public init(cwd: String, projectId: String? = nil, modelId: String? = nil, provider: String? = nil, title: String? = nil) {
        self.cwd = cwd
        self.projectId = projectId
        self.modelId = modelId
        self.provider = provider
        self.title = title
    }

    enum CodingKeys: String, CodingKey {
        case cwd
        case projectId = "projectId"
        case modelId   = "modelId"
        case provider
        case title
    }
}

public struct UpdateSessionDto: Codable, Sendable {
    public let title: String?
    public let modelId: String?
    public let provider: String?

    public init(title: String? = nil, modelId: String? = nil, provider: String? = nil) {
        self.title = title
        self.modelId = modelId
        self.provider = provider
    }

    enum CodingKeys: String, CodingKey {
        case title
        case modelId = "modelId"
        case provider
    }
}

public struct RunPromptDto: Codable, Sendable {
    public let prompt: String
    public let modelId: String?
    public let provider: String?
    public let approvalMode: String?

    public init(prompt: String, modelId: String? = nil, provider: String? = nil, approvalMode: String? = nil) {
        self.prompt = prompt
        self.modelId = modelId
        self.provider = provider
        self.approvalMode = approvalMode
    }

    enum CodingKeys: String, CodingKey {
        case prompt
        case modelId       = "modelId"
        case provider
        case approvalMode  = "approvalMode"
    }
}

public struct OAuthLoginUrlDto: Codable, Sendable {
    public let provider: String

    public init(provider: String) { self.provider = provider }
}

public struct OAuthCallbackDto: Codable, Sendable {
    public let provider: String
    public let code: String
    public let state: String?

    public init(provider: String, code: String, state: String? = nil) {
        self.provider = provider
        self.code = code
        self.state = state
    }
}

public struct AnswerQuestionDto: Codable, Sendable {
    public let requestId: String
    public let answer: JSONValue

    public init(requestId: String, answer: JSONValue) {
        self.requestId = requestId
        self.answer = answer
    }

    enum CodingKeys: String, CodingKey {
        case requestId = "requestId"
        case answer
    }
}

public struct ApproveToolPermissionDto: Codable, Sendable {
    public let requestId: String
    public let allow: Bool

    public init(requestId: String, allow: Bool) {
        self.requestId = requestId
        self.allow = allow
    }

    enum CodingKeys: String, CodingKey {
        case requestId = "requestId"
        case allow
    }
}

// MARK: - Response DTOs (mirror packages/types/src/api.ts)

public struct ProjectInfo: Codable, Sendable, Hashable, Identifiable {
    public let id: String
    public let name: String
    public let path: String
    public let createdAt: Double
    public let updatedAt: Double

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case path
        case createdAt   = "createdAt"
        case updatedAt   = "updatedAt"
    }
}

public struct AuthProviderStatus: Codable, Sendable, Hashable {
    public let loggedIn: Bool
    public let email: String?
    public let projectId: String?
}

public struct AuthStatusResponse: Codable, Sendable, Hashable {
    public let gemini: AuthProviderStatus
    public let antigravity: AuthProviderStatus
}

public struct FsTreeEntry: Codable, Sendable, Hashable, Identifiable {
    public var id: String { path }
    public let name: String
    public let path: String
    public let isDir: Bool
    public let size: Int64?
    public let children: [FsTreeEntry]?

    enum CodingKeys: String, CodingKey {
        case name
        case path
        case isDir   = "isDir"
        case size
        case children
    }
}
