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

    /// Decode accumulated data lines into events and call `onEvent` for each.
    ///
    /// The server emits one JSON event per `data:` line. When the SSE
    /// blank-line event separators are not surfaced by the line streamer
    /// (observed with `URL.AsyncBytes.lines`), multiple events accumulate
    /// here. We therefore try decoding each line on its own first, and only
    /// fall back to the spec-compliant joined payload (genuine multi-line
    /// `data:` fields) for any lines that did not decode individually.
    /// Surfaces ultimate decoding failures as synthetic error events
    /// instead of silently dropping them.
    private func flushDataLines(
        _ dataLines: inout [String],
        onEvent: @escaping @Sendable (AgentSessionEvent) -> Void
    ) {
        guard !dataLines.isEmpty else { return }
        defer { dataLines.removeAll() }

        var leftovers: [String] = []
        for line in dataLines {
            guard let jsonData = line.data(using: .utf8) else {
                leftovers.append(line)
                continue
            }
            if let event = try? JSONDecoder().decode(AgentSessionEvent.self, from: jsonData) {
                onEvent(event)
            } else {
                leftovers.append(line)
            }
        }

        guard !leftovers.isEmpty else { return }
        let payload = leftovers.joined(separator: "\n")
        guard let jsonData = payload.data(using: .utf8) else { return }

        do {
            let event = try JSONDecoder().decode(AgentSessionEvent.self, from: jsonData)
            onEvent(event)
        } catch {
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
