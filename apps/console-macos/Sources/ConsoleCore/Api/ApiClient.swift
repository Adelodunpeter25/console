import Foundation

/// Mirrors api/client.rs — thin async HTTP wrapper around URLSession that
/// unwraps the `ApiResponse<T>` envelope and returns `data`.
public final class ApiClient: @unchecked Sendable {

    public let session: URLSession

    public init(session: URLSession = .shared) {
        self.session = session
    }

    // MARK: - Generic request helpers

    public func get<T: Decodable>(_ path: String, query: [URLQueryItem] = []) async throws -> T {
        try await request("GET", path, query: query, body: nil as Data?)
    }

    public func post<T: Decodable, B: Encodable>(_ path: String, body: B) async throws -> T {
        try await request("POST", path, body: body)
    }

    public func post<T: Decodable>(_ path: String) async throws -> T {
        try await request("POST", path, body: nil as Data?)
    }

    public func patch<T: Decodable, B: Encodable>(_ path: String, body: B) async throws -> T {
        try await request("PATCH", path, body: body)
    }

    public func delete<T: Decodable>(_ path: String, query: [URLQueryItem] = []) async throws -> T {
        try await request("DELETE", path, query: query, body: nil as Data?)
    }

    // MARK: - Core

    private func request<T: Decodable, B: Encodable>(
        _ method: String,
        _ path: String,
        query: [URLQueryItem] = [],
        body: B?
    ) async throws -> T {
        var components = URLComponents(string: "\(ConsoleConfig.apiBase)\(path)")
        if !query.isEmpty { components?.queryItems = query }
        guard let url = components?.url else {
            throw AppError.config("Invalid URL for path: \(path)")
        }

        var req = URLRequest(url: url)
        req.httpMethod = method

        if let body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try JSONEncoder().encode(body)
        }

        let (data, response): (Data, URLResponse)
        do {
            (data, response) = try await session.data(for: req)
        } catch {
            throw AppError.http(error.localizedDescription)
        }

        guard let http = response as? HTTPURLResponse else {
            throw AppError.server("Non-HTTP response")
        }

        guard (200..<300).contains(http.statusCode) else {
            let bodyText = String(data: data, encoding: .utf8) ?? ""
            throw AppError.server("HTTP \(http.statusCode): \(bodyText)")
        }

        // Unwrap ApiResponse<T> envelope
        let envelope: ApiResponse<T>
        do {
            envelope = try JSONDecoder().decode(ApiResponse<T>.self, from: data)
        } catch {
            throw AppError.json(error.localizedDescription)
        }

        guard envelope.success else {
            throw AppError.api(envelope.error ?? "Unknown API error")
        }

        guard let payload = envelope.data else {
            throw AppError.api("Response contained no data")
        }

        return payload
    }

    // MARK: - Raw SSE stream (used by Run.swift)

    /// Returns the raw `URLSession.AsyncBytes` for an SSE endpoint, after
    /// checking the HTTP status. Caller is responsible for parsing SSE frames.
    public func sseStream(
        _ path: String,
        method: String = "POST",
        body: Data? = nil
    ) async throws -> (URLSession.AsyncBytes, HTTPURLResponse) {
        guard let url = URL(string: "\(ConsoleConfig.apiBase)\(path)") else {
            throw AppError.config("Invalid URL for path: \(path)")
        }

        var req = URLRequest(url: url)
        req.httpMethod = method
        req.setValue("text/event-stream", forHTTPHeaderField: "Accept")
        if let body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = body
        }

        let (bytes, response): (URLSession.AsyncBytes, URLResponse)
        do {
            (bytes, response) = try await session.bytes(for: req)
        } catch {
            throw AppError.http(error.localizedDescription)
        }

        guard let http = response as? HTTPURLResponse else {
            throw AppError.server("Non-HTTP response")
        }
        guard (200..<300).contains(http.statusCode) else {
            throw AppError.server("HTTP \(http.statusCode)")
        }

        return (bytes, http)
    }

    public var baseURL: String { ConsoleConfig.serverURL }
}
