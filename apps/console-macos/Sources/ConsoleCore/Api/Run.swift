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

        // Parse SSE at the byte level per the spec. A stream of UTF-8 bytes is
        // accumulated into a buffer and split on blank-line boundaries
        // (`\n\n` or `\r\n\r\n`), each of which delimits one event frame.
        // Within a frame, `data:` lines are joined with `\n` to form the
        // payload; `event:`, `id:`, and `retry:` lines are recognized and
        // ignored (the event type is embedded in the JSON payload itself).
        //
        // This avoids the fragility of `URL.AsyncBytes.lines`, which does not
        // reliably surface blank-line separators and caused multiple events
        // to be merged into one decode attempt.
        var buffer = Data()
        var frame = SseFrame()

        for try await byte in bytes {
            buffer.append(byte)

            // Extract and process every complete line in the buffer.
            while let lineEnd = buffer.firstIndex(of: 0x0A) {
                let lineData = buffer[buffer.startIndex..<lineEnd]
                buffer.removeSubrange(buffer.startIndex...lineEnd)

                // Strip a trailing CR for CRLF line endings.
                var slice = lineData
                if slice.last == 0x0D { slice = slice.dropLast() }

                let line = String(data: slice, encoding: .utf8) ?? ""
                let trimmed = line.trimmingCharacters(in: .whitespaces)

                if trimmed.isEmpty {
                    // Blank line = event boundary. Flush the accumulated frame.
                    if !frame.dataLines.isEmpty {
                        flushFrame(frame, onEvent: onEvent)
                        frame = SseFrame()
                    }
                    continue
                }

                // Comment line (SSE spec: lines starting with ':').
                if trimmed.hasPrefix(":") { continue }

                if let field = SseField(line) {
                    switch field {
                    case .data(let value):
                        frame.dataLines.append(value)
                    case .event, .id, .retry:
                        // Acknowledged but unused — the JSON payload carries
                        // the event type, and we don't track retry/id state.
                        break
                    }
                } else {
                    // Unknown field — per spec, ignore.
                    continue
                }
            }
        }

        // Flush any trailing frame when the stream closes without a final
        // blank line.
        if !frame.dataLines.isEmpty {
            flushFrame(frame, onEvent: onEvent)
        }
    }

    /// Decode one SSE frame's data payload into events and call `onEvent`.
    ///
    /// The server emits one JSON event per `data:` line. To stay robust
    /// against both single-line and spec-compliant multi-line `data:` fields,
    /// we first try decoding each data line on its own; any line that does
    /// not decode individually is joined with the others for a final
    /// whole-payload attempt. Ultimate decode failures surface as synthetic
    /// error events so the UI can show them instead of silently dropping.
    private func flushFrame(
        _ frame: SseFrame,
        onEvent: @escaping @Sendable (AgentSessionEvent) -> Void
    ) {
        var leftovers: [String] = []
        for line in frame.dataLines {
            guard let jsonData = line.data(using: .utf8) else {
                leftovers.append(line)
                continue
            }
            do {
                let event = try JSONDecoder.agent.decode(AgentSessionEvent.self, from: jsonData)
                onEvent(event)
            } catch let error as UnknownEventTypeError {
                // Unknown event type from a newer backend — skip silently
                // rather than surfacing as a hard error, so the stream keeps
                // flowing for the events we do understand.
                #if DEBUG
                print("[SSE] Skipping unknown event: \(error.typeName)")
                #endif
            } catch {
                // Malformed payload for a known type — retry as part of the
                // joined payload below (genuine multi-line `data:` fields).
                leftovers.append(line)
            }
        }

        guard !leftovers.isEmpty else { return }
        let payload = leftovers.joined(separator: "\n")
        guard let jsonData = payload.data(using: .utf8) else { return }

        do {
            let event = try JSONDecoder.agent.decode(AgentSessionEvent.self, from: jsonData)
            onEvent(event)
        } catch let error as UnknownEventTypeError {
            #if DEBUG
            print("[SSE] Skipping unknown event: \(error.typeName)")
            #endif
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

// MARK: - SSE parsing primitives

/// Accumulated fields for a single SSE event frame.
private struct SseFrame {
    var dataLines: [String] = []
}

/// A parsed SSE field line (`data:`, `event:`, `id:`, `retry:`).
private enum SseField {
    case data(String)
    case event(String)
    case id(String)
    case retry(String)

    init?(_ line: String) {
        // SSE spec: one colon; an optional single space after the colon is
        // stripped from the value. Lines without a colon are treated as field
        // names with an empty value.
        let name: String
        let rawValue: String

        if let colonIndex = line.firstIndex(of: ":") {
            name = String(line[line.startIndex..<colonIndex])
            var valueStart = line.index(after: colonIndex)
            // Strip one leading space (U+0020) only.
            if valueStart < line.endIndex, line[valueStart] == " " {
                valueStart = line.index(after: valueStart)
            }
            rawValue = String(line[valueStart..<line.endIndex])
        } else {
            name = line
            rawValue = ""
        }

        switch name {
        case "data":
            self = .data(rawValue)
        case "event":
            self = .event(rawValue)
        case "id":
            self = .id(rawValue)
        case "retry":
            self = .retry(rawValue)
        default:
            return nil
        }
    }
}

// MARK: - Shared decoder

extension JSONDecoder {
    /// A shared decoder configured for agent event payloads. Reusing a single
    /// decoder avoids repeated allocation on every SSE frame and keeps key
    /// decoding consistent across the stream.
    static let agent: JSONDecoder = {
        let decoder = JSONDecoder()
        return decoder
    }()
}
