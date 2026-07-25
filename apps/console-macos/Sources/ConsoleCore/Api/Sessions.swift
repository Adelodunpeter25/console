import Foundation

// Mirrors api/sessions.rs

public extension ApiClient {
    func listSessions(cwd: String? = nil, projectId: String? = nil) async throws -> [SessionHeader] {
        var query: [URLQueryItem] = []
        if let cwd { query.append(.init(name: "cwd", value: cwd)) }
        if let projectId { query.append(.init(name: "projectId", value: projectId)) }
        return try await get("/sessions", query: query)
    }

    func createSession(_ dto: CreateSessionDto) async throws -> SessionHeader {
        try await post("/sessions", body: dto)
    }

    func getSession(id: String) async throws -> SessionDetailResponse {
        try await get("/sessions/\(id)")
    }

    func updateSession(id: String, _ dto: UpdateSessionDto) async throws -> SessionHeader {
        try await patch("/sessions/\(id)", body: dto)
    }

    func deleteSession(id: String) async throws -> JSONValue {
        try await delete("/sessions/\(id)")
    }
}
