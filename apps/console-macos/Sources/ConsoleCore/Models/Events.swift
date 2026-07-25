import Foundation

// MARK: - AskQuestionRequest  (mirrors models/events.rs)

public struct AskQuestionRequest: Codable, Sendable, Hashable {
    public let requestId: String
    public let question: String
    public let options: [String]
    public let isMultiSelect: Bool?

    enum CodingKeys: String, CodingKey {
        case requestId   = "requestId"
        case question
        case options
        case isMultiSelect = "isMultiSelect"
    }
}

// MARK: - ModelStreamPartData

public struct ModelStreamPartData: Codable, Sendable, Hashable {
    public let text: String?
    public let thinking: String?
    public let toolCall: ToolCall?

    enum CodingKeys: String, CodingKey {
        case text
        case thinking
        case toolCall = "toolCall"
    }
}

// MARK: - AgentSessionError

public struct AgentSessionError: Codable, Sendable, Hashable {
    public let message: String
    public let data: JSONValue?
}

// MARK: - AgentSessionEvent
// Mirrors models/events.rs — serde(tag = "type", rename_all = "camelCase")

/// Thrown when an event payload carries a `type` the client does not yet
/// recognize. The SSE parser catches this and skips the event (with a debug
/// log) instead of surfacing it as a hard error, so newer backend event
/// types don't break older clients.
public struct UnknownEventTypeError: Error, CustomStringConvertible {
    public let typeName: String
    public var description: String { "Unknown event type: \(typeName)" }
}

public enum AgentSessionEvent: Codable, Sendable, Hashable {
    case sessionStart
    case turnStart(prompt: String)
    case modelStreamStart(turnId: String)
    case modelStreamPart(part: ModelStreamPartData)
    case modelStreamEnd(turn: AssistantMessage)
    case toolExecutionStart(calls: [ToolCall])
    case permissionRequest(request: PermissionRequest)
    case askQuestion(request: AskQuestionRequest)
    case toolExecutionResult(result: ToolResult)
    case toolExecutionEnd(results: [ToolResult])
    case compaction(summary: String, originalMessageCount: UInt64)
    case turnEnd(turnId: String)
    case sessionEnd
    case error(error: AgentSessionError)

    private enum CodingKeys: String, CodingKey {
        case type
        case prompt
        case turnId      = "turnId"
        case part
        case turn
        case calls
        case request
        case result
        case results
        case summary
        case originalMessageCount = "originalMessageCount"
        case error
    }

    private enum EventType: String, Codable {
        case sessionStart         = "sessionStart"
        case turnStart            = "turnStart"
        case modelStreamStart     = "modelStreamStart"
        case modelStreamPart      = "modelStreamPart"
        case modelStreamEnd       = "modelStreamEnd"
        case toolExecutionStart   = "toolExecutionStart"
        case permissionRequest    = "permissionRequest"
        case askQuestion          = "askQuestion"
        case toolExecutionResult  = "toolExecutionResult"
        case toolExecutionEnd     = "toolExecutionEnd"
        case compaction           = "compaction"
        case turnEnd              = "turnEnd"
        case sessionEnd           = "sessionEnd"
        case error                = "error"
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        // Decode the type tag as a raw string first so unknown types throw a
        // typed `UnknownEventTypeError` (skippable) rather than a generic
        // decoding error (which would surface as a hard error to the UI).
        let typeRaw = try c.decode(String.self, forKey: .type)
        guard let type = EventType(rawValue: typeRaw) else {
            throw UnknownEventTypeError(typeName: typeRaw)
        }
        switch type {
        case .sessionStart:
            self = .sessionStart
        case .turnStart:
            self = .turnStart(prompt: try c.decode(String.self, forKey: .prompt))
        case .modelStreamStart:
            self = .modelStreamStart(turnId: try c.decode(String.self, forKey: .turnId))
        case .modelStreamPart:
            self = .modelStreamPart(part: try c.decode(ModelStreamPartData.self, forKey: .part))
        case .modelStreamEnd:
            self = .modelStreamEnd(turn: try c.decode(AssistantMessage.self, forKey: .turn))
        case .toolExecutionStart:
            self = .toolExecutionStart(calls: try c.decode([ToolCall].self, forKey: .calls))
        case .permissionRequest:
            self = .permissionRequest(request: try c.decode(PermissionRequest.self, forKey: .request))
        case .askQuestion:
            self = .askQuestion(request: try c.decode(AskQuestionRequest.self, forKey: .request))
        case .toolExecutionResult:
            self = .toolExecutionResult(result: try c.decode(ToolResult.self, forKey: .result))
        case .toolExecutionEnd:
            self = .toolExecutionEnd(results: try c.decode([ToolResult].self, forKey: .results))
        case .compaction:
            self = .compaction(
                summary: try c.decode(String.self, forKey: .summary),
                originalMessageCount: try c.decode(UInt64.self, forKey: .originalMessageCount)
            )
        case .turnEnd:
            self = .turnEnd(turnId: try c.decode(String.self, forKey: .turnId))
        case .sessionEnd:
            self = .sessionEnd
        case .error:
            self = .error(error: try c.decode(AgentSessionError.self, forKey: .error))
        }
    }

    public func encode(to encoder: any Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        switch self {
        case .sessionStart:
            try c.encode(EventType.sessionStart, forKey: .type)
        case .turnStart(let prompt):
            try c.encode(EventType.turnStart, forKey: .type)
            try c.encode(prompt, forKey: .prompt)
        case .modelStreamStart(let turnId):
            try c.encode(EventType.modelStreamStart, forKey: .type)
            try c.encode(turnId, forKey: .turnId)
        case .modelStreamPart(let part):
            try c.encode(EventType.modelStreamPart, forKey: .type)
            try c.encode(part, forKey: .part)
        case .modelStreamEnd(let turn):
            try c.encode(EventType.modelStreamEnd, forKey: .type)
            try c.encode(turn, forKey: .turn)
        case .toolExecutionStart(let calls):
            try c.encode(EventType.toolExecutionStart, forKey: .type)
            try c.encode(calls, forKey: .calls)
        case .permissionRequest(let request):
            try c.encode(EventType.permissionRequest, forKey: .type)
            try c.encode(request, forKey: .request)
        case .askQuestion(let request):
            try c.encode(EventType.askQuestion, forKey: .type)
            try c.encode(request, forKey: .request)
        case .toolExecutionResult(let result):
            try c.encode(EventType.toolExecutionResult, forKey: .type)
            try c.encode(result, forKey: .result)
        case .toolExecutionEnd(let results):
            try c.encode(EventType.toolExecutionEnd, forKey: .type)
            try c.encode(results, forKey: .results)
        case .compaction(let summary, let count):
            try c.encode(EventType.compaction, forKey: .type)
            try c.encode(summary, forKey: .summary)
            try c.encode(count, forKey: .originalMessageCount)
        case .turnEnd(let turnId):
            try c.encode(EventType.turnEnd, forKey: .type)
            try c.encode(turnId, forKey: .turnId)
        case .sessionEnd:
            try c.encode(EventType.sessionEnd, forKey: .type)
        case .error(let error):
            try c.encode(EventType.error, forKey: .type)
            try c.encode(error, forKey: .error)
        }
    }
}

// MARK: - SseEventFrame

public struct SseEventFrame: Codable, Sendable, Hashable {
    public let event: String
    public let data: AgentSessionEvent
}
