import Foundation

// MARK: - ToolTier  (mirrors models/tool.rs — serde rename_all = "kebab-case")

public enum ToolTier: String, Codable, Sendable {
    case read   = "read"
    case write  = "write"
    case exec   = "exec"
}

// MARK: - ApprovalMode  (kebab-case)

public enum ApprovalMode: String, Codable, Sendable {
    case alwaysAsk    = "always-ask"
    case acceptEdits  = "accept-edits"
    case planMode     = "plan-mode"
    case fullAccess   = "full-access"
}

// MARK: - ApprovalPolicy  (lowercase)

public enum ApprovalPolicy: String, Codable, Sendable {
    case allow  = "allow"
    case deny   = "deny"
    case prompt = "prompt"
}

// MARK: - PermissionRequest

public struct PermissionRequest: Codable, Sendable, Hashable {
    public let requestId: String
    public let toolCallId: String
    public let toolName: String
    public let args: JSONValue
    public let tier: ToolTier
    public let reason: String?

    enum CodingKeys: String, CodingKey {
        case requestId = "requestId"
        case toolCallId = "toolCallId"
        case toolName = "toolName"
        case args
        case tier
        case reason
    }
}

// MARK: - ToolCall

public struct ToolCall: Codable, Sendable, Hashable {
    public let id: String
    public let name: String
    public let arguments: JSONValue
}

// MARK: - ToolResult

public struct ToolResult: Codable, Sendable, Hashable {
    public let toolCallId: String
    public let toolName: String?
    public let content: JSONValue
    public let isError: Bool?

    enum CodingKeys: String, CodingKey {
        case toolCallId = "toolCallId"
        case toolName = "toolName"
        case content
        case isError = "isError"
    }
}
