import Foundation

// MARK: - AssistantMessageContent
// Mirrors models/agent.rs — serde(tag = "type", rename_all = "camelCase")
// Variants: text, thinking, toolCall

public enum AssistantMessageContent: Codable, Sendable, Hashable {
    case text(text: String)
    case thinking(text: String)
    case toolCall(call: ToolCall)

    private enum CodingKeys: String, CodingKey {
        case type
        case text
        case thinking
        case call
    }

    private enum ContentType: String, Codable {
        case text       = "text"
        case thinking   = "thinking"
        case toolCall   = "toolCall"
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let type = try c.decode(ContentType.self, forKey: .type)
        switch type {
        case .text:
            self = .text(text: try c.decode(String.self, forKey: .text))
        case .thinking:
            self = .thinking(text: try c.decode(String.self, forKey: .thinking))
        case .toolCall:
            self = .toolCall(call: try c.decode(ToolCall.self, forKey: .call))
        }
    }

    public func encode(to encoder: any Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        switch self {
        case .text(let text):
            try c.encode(ContentType.text, forKey: .type)
            try c.encode(text, forKey: .text)
        case .thinking(let text):
            try c.encode(ContentType.thinking, forKey: .type)
            try c.encode(text, forKey: .thinking)
        case .toolCall(let call):
            try c.encode(ContentType.toolCall, forKey: .type)
            try c.encode(call, forKey: .call)
        }
    }
}

// MARK: - AgentMessage
// Mirrors models/agent.rs — serde(tag = "role", rename_all = "camelCase")
// Variants: user, assistant, toolResult

public enum AgentMessage: Codable, Sendable, Hashable {
    case user(content: String)
    case assistant(id: String?, content: [AssistantMessageContent], stopReason: String?)
    case toolResult(results: [ToolResult])

    private enum CodingKeys: String, CodingKey {
        case role
        case content
        case id
        case stopReason
        case results
    }

    private enum RoleType: String, Codable {
        case user       = "user"
        case assistant  = "assistant"
        case toolResult = "toolResult"
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let role = try c.decode(RoleType.self, forKey: .role)
        switch role {
        case .user:
            self = .user(content: try c.decode(String.self, forKey: .content))
        case .assistant:
            self = .assistant(
                id: try c.decodeIfPresent(String.self, forKey: .id),
                content: try c.decode([AssistantMessageContent].self, forKey: .content),
                stopReason: try c.decodeIfPresent(String.self, forKey: .stopReason)
            )
        case .toolResult:
            self = .toolResult(results: try c.decode([ToolResult].self, forKey: .results))
        }
    }

    public func encode(to encoder: any Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        switch self {
        case .user(let content):
            try c.encode(RoleType.user, forKey: .role)
            try c.encode(content, forKey: .content)
        case .assistant(let id, let content, let stopReason):
            try c.encode(RoleType.assistant, forKey: .role)
            try c.encodeIfPresent(id, forKey: .id)
            try c.encode(content, forKey: .content)
            try c.encodeIfPresent(stopReason, forKey: .stopReason)
        case .toolResult(let results):
            try c.encode(RoleType.toolResult, forKey: .role)
            try c.encode(results, forKey: .results)
        }
    }
}

// MARK: - Convenience structs (for non-tagged usage)

public struct UserMessage: Codable, Sendable, Hashable {
    public let role: String
    public let content: String
}

public struct AssistantMessage: Codable, Sendable, Hashable {
    public let role: String
    public let id: String?
    public let content: [AssistantMessageContent]
    public let stopReason: String?

    enum CodingKeys: String, CodingKey {
        case role
        case id
        case content
        case stopReason
    }
}

public struct ToolResultMessage: Codable, Sendable, Hashable {
    public let role: String
    public let results: [ToolResult]
}
