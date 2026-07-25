import Foundation

// Mirrors api/fs.rs

public extension ApiClient {
    func browseDirectory(path: String? = nil) async throws -> [FsTreeEntry] {
        var query: [URLQueryItem] = []
        if let path { query.append(.init(name: "path", value: path)) }
        return try await get("/fs/browse", query: query)
    }

    func pickFolder() async throws -> JSONValue {
        try await post("/fs/pick-folder")
    }

    func getDirectoryTree(path: String? = nil, depth: Int? = nil) async throws -> JSONValue {
        var query: [URLQueryItem] = []
        if let path { query.append(.init(name: "path", value: path)) }
        if let depth { query.append(.init(name: "depth", value: String(depth))) }
        return try await get("/fs/tree", query: query)
    }

    func readFile(path: String, startLine: Int? = nil, endLine: Int? = nil) async throws -> JSONValue {
        var query: [URLQueryItem] = [.init(name: "path", value: path)]
        if let s = startLine { query.append(.init(name: "startLine", value: String(s))) }
        if let e = endLine { query.append(.init(name: "endLine", value: String(e))) }
        return try await get("/fs/file", query: query)
    }

    func writeFile(path: String, content: String) async throws -> JSONValue {
        struct Body: Encodable { let path: String; let content: String }
        return try await post("/fs/file", body: Body(path: path, content: content))
    }

    func deleteFile(path: String) async throws -> JSONValue {
        try await delete("/fs/file", query: [.init(name: "path", value: path)])
    }

    func createDirectory(path: String) async throws -> JSONValue {
        struct Body: Encodable { let path: String }
        return try await post("/fs/dir", body: Body(path: path))
    }

    func deleteDirectory(path: String) async throws -> JSONValue {
        try await delete("/fs/dir", query: [.init(name: "path", value: path)])
    }
}
