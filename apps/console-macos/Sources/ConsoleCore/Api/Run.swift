import Foundation

// Mirrors api/run.rs — SSE streaming for agent runs

public extension ApiClient {

    /// Streams agent events from `POST /sessions/{id}/run`.
    /// Calls `onEvent` for each decoded `AgentSessionEvent`.
    func runAgentStream(
        sessionId: String,
        dto: RunPromptDto,
        onEvent: @escaping @Sendable (AgentSessionEvent) -> Void
    ) async throws {
        let bodyData = try JSONEncoder().encode(dto)
        let (bytes, _) = try await sseStream(
            "/sessions/\(sessionId)/run",
            method: "POST",
            body: bodyData
        )

        // Parse SSE: lines starting with "data: " are accumulated until a
        // blank line, then the joined payload is decoded as an event.
        var dataLines: [String] = []

        for try await line in bytes.lines {
            let trimmed = line.trimmingCharacters(in: .whitespaces)

            if trimmed.isEmpty {
                if !dataLines.isEmpty {
                    let payload = dataLines.joined(separator: "\n")
                    if let jsonData = payload.data(using: .utf8) {
                        if let event = try? JSONDecoder().decode(AgentSessionEvent.self, from: jsonData) {
                            onEvent(event)
                        }
                    }
                    dataLines.removeAll()
                }
                continue
            }

            if trimmed.hasPrefix("data: ") {
                dataLines.append(String(trimmed.dropFirst(6)))
            }
        }
    }

    /// Aborts a running agent session.
    func abortRun(sessionId: String) async throws -> JSONValue {
        struct EmptyBody: Encodable {}
        return try await post("/sessions/\(sessionId)/abort", body: EmptyBody())
    }

    /// Answers a question posed by the agent during a run.
    func answerQuestion(sessionId: String, requestId: String, answer: JSONValue) async throws -> JSONValue {
        let dto = AnswerQuestionDto(requestId: requestId, answer: answer)
        return try await post("/sessions/\(sessionId)/answer", body: dto)
    }

    /// Approves or denies a tool permission request.
    func approveToolPermission(sessionId: String, requestId: String, allow: Bool) async throws -> JSONValue {
        let dto = ApproveToolPermissionDto(requestId: requestId, allow: allow)
        return try await post("/sessions/\(sessionId)/approve", body: dto)
    }
}
