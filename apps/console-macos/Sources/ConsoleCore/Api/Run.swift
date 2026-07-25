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

        // Parse SSE: lines starting with "data:" are accumulated until a
        // blank line, then the joined payload is decoded as an event.
        var dataLines: [String] = []

        for try await line in bytes.lines {
            let trimmed = line.trimmingCharacters(in: .whitespaces)

            if trimmed.isEmpty {
                flushDataLines(&dataLines, onEvent: onEvent)
                continue
            }

            // Handle both "data: " and "data:" (SSE spec allows optional space)
            if trimmed.hasPrefix("data: ") {
                dataLines.append(String(trimmed.dropFirst(6)))
            } else if trimmed.hasPrefix("data:") {
                dataLines.append(String(trimmed.dropFirst(5)).trimmingCharacters(in: .whitespaces))
            }
        }

        // Flush any remaining data lines when the stream ends (the server
        // may close the connection without a trailing blank line).
        flushDataLines(&dataLines, onEvent: onEvent)
    }

    /// Decode accumulated data lines into an event and call onEvent.
    /// Surfaces decoding errors as synthetic error events instead of
    /// silently dropping them.
    private func flushDataLines(
        _ dataLines: inout [String],
        onEvent: @escaping @Sendable (AgentSessionEvent) -> Void
    ) {
        guard !dataLines.isEmpty else { return }
        let payload = dataLines.joined(separator: "\n")
        dataLines.removeAll()

        guard let jsonData = payload.data(using: .utf8) else { return }

        do {
            let event = try JSONDecoder().decode(AgentSessionEvent.self, from: jsonData)
            onEvent(event)
        } catch {
            // Surface decoding errors so the UI can show them instead of
            // silently dropping the event.
            #if DEBUG
            print("[SSE] Failed to decode event: \(error)\n  payload: \(payload)")
            #endif
            let errorEvent = AgentSessionEvent.error(
                error: AgentSessionError(
                    message: "Failed to decode server event: \(error.localizedDescription)",
                    data: nil
                )
            )
            onEvent(errorEvent)
        }
    }

    /// Aborts a running agent session.
    func abortRun(sessionId: String) async throws -> JSONValue {
        struct EmptyBody: Encodable, Sendable {}
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
