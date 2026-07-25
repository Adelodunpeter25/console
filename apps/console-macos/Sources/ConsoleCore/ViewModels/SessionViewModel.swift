import Foundation
import SwiftUI

/// View model for a single chat session. Replaces the old `SessionModel` —
/// loads messages, sends prompts via HTTP/SSE, and exposes streaming state.
@MainActor
public final class SessionViewModel: ObservableObject {

    public let client: ApiClient
    public let sessionId: String

    // MARK: - Published state

    @Published public var header: SessionHeader?
    @Published public var messages: [AgentMessage] = []
    @Published public var streamingText: String = ""
    @Published public var streamingThinking: String = ""
    @Published public var isStreaming = false
    @Published public var errorMessage: String?
    @Published public var pendingPermission: PermissionRequest?
    @Published public var pendingQuestion: AskQuestionRequest?

    // Accumulated assistant turn during streaming
    private var currentAssistantParts: [AssistantMessageContent] = []

    public init(client: ApiClient, sessionId: String) {
        self.client = client
        self.sessionId = sessionId
    }

    // MARK: - Load

    public func loadDetails() async {
        do {
            let detail = try await client.getSession(id: sessionId)
            header = detail.header
            messages = detail.messages
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    // MARK: - Send prompt

    public func sendPrompt(_ prompt: String, modelId: String? = nil, provider: String? = nil, approvalMode: ApprovalMode? = nil) async {
        guard !isStreaming else { return }

        // Optimistically add the user message
        messages.append(.user(content: prompt))

        isStreaming = true
        streamingText = ""
        streamingThinking = ""
        currentAssistantParts = []
        errorMessage = nil

        let dto = RunPromptDto(
            prompt: prompt,
            modelId: modelId,
            provider: provider,
            approvalMode: approvalMode?.rawValue
        )

        // Capture a sendable handler that dispatches to MainActor.
        // Bind `self` to a local `let` before the Task so we don't reference
        // the captured `weak var` from concurrently-executing code.
        let handle: @Sendable (AgentSessionEvent) -> Void = { [weak self] event in
            guard let self else { return }
            Task { @MainActor in
                self.handleEvent(event)
            }
        }

        do {
            try await client.runAgentStream(sessionId: sessionId, dto: dto, onEvent: handle)
            // Stream ended — finalize the assistant turn
            finalizeAssistantTurn()
        } catch {
            errorMessage = error.localizedDescription
            finalizeAssistantTurn()
        }

        isStreaming = false
        streamingText = ""
        streamingThinking = ""
    }

    // MARK: - Abort

    public func abort() async {
        do {
            _ = try await client.abortRun(sessionId: sessionId)
        } catch {
            errorMessage = error.localizedDescription
        }
        isStreaming = false
        finalizeAssistantTurn()
        streamingText = ""
        streamingThinking = ""
    }

    // MARK: - Answer question

    public func answerQuestion(_ answer: String) async {
        guard let question = pendingQuestion else { return }
        do {
            _ = try await client.answerQuestion(
                sessionId: sessionId,
                requestId: question.requestId,
                answer: .string(answer)
            )
            pendingQuestion = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    // MARK: - Approve tool

    public func approveTool(allow: Bool) async {
        guard let perm = pendingPermission else { return }
        do {
            _ = try await client.approveToolPermission(
                sessionId: sessionId,
                requestId: perm.requestId,
                allow: allow
            )
            pendingPermission = nil
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    // MARK: - Event handling

    private func handleEvent(_ event: AgentSessionEvent) {
        switch event {
        case .sessionStart:
            break

        case .turnStart:
            currentAssistantParts = []
            streamingText = ""

        case .modelStreamStart:
            break

        case .modelStreamPart(let part):
            if let text = part.text {
                streamingText += text
            }
            if let thinking = part.thinking {
                streamingThinking += thinking
            }
            if let toolCall = part.toolCall {
                currentAssistantParts.append(.toolCall(call: toolCall))
            }

        case .modelStreamEnd(let turn):
            // The server sends the complete assistant message
            if !streamingText.isEmpty && !currentAssistantParts.contains(where: { if case .text = $0 { true } else { false } }) {
                currentAssistantParts.insert(.text(text: streamingText), at: 0)
            }
            messages.append(.assistant(
                id: turn.id,
                content: turn.content.isEmpty ? currentAssistantParts : turn.content,
                stopReason: turn.stopReason
            ))
            currentAssistantParts = []
            streamingText = ""

        case .toolExecutionStart:
            break

        case .permissionRequest(let request):
            pendingPermission = request

        case .askQuestion(let request):
            pendingQuestion = request

        case .toolExecutionResult(let result):
            // Append as a tool result message
            messages.append(.toolResult(results: [result]))

        case .toolExecutionEnd(let results):
            // If multiple results came in a batch, add them
            if !results.isEmpty {
                messages.append(.toolResult(results: results))
            }

        case .compaction:
            // Compaction replaces history — reload from server
            Task { await loadDetails() }

        case .turnEnd:
            finalizeAssistantTurn()

        case .sessionEnd:
            isStreaming = false

        case .error(let agentError):
            errorMessage = agentError.message
            finalizeAssistantTurn()
            isStreaming = false
        }
    }

    private func finalizeAssistantTurn() {
        guard !currentAssistantParts.isEmpty || !streamingText.isEmpty else { return }

        if !streamingText.isEmpty {
            currentAssistantParts.insert(.text(text: streamingText), at: 0)
        }

        messages.append(.assistant(
            id: nil,
            content: currentAssistantParts,
            stopReason: nil
        ))
        currentAssistantParts = []
        streamingText = ""
    }
}
