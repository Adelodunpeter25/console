import Foundation

// Mirrors api/projects.rs

public extension ApiClient {
    func listProjects() async throws -> [ProjectInfo] {
        try await get("/projects")
    }

    func addProject(path: String) async throws -> ProjectInfo {
        struct Body: Encodable, Sendable { let path: String }
        return try await post("/projects", body: Body(path: path))
    }
}
