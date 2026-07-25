import Foundation

/// Central place for building backend URLs.
///
/// All request URL construction lives here so path/query assembly is consistent
/// across the API client, SSE streamer, and any other caller. `ConsoleConfig`
/// holds the mutable server origin; `ConsoleURLs` turns a path + query into a
/// concrete `URL`.
public enum ConsoleURLs {

    /// The API path prefix. Every REST endpoint is served under `/api`.
    public static let apiPrefix = "/api"

    /// Builds a full `URL` for an API path (prefixed with `/api`).
    /// - Parameters:
    ///   - path: Path beginning with `/` (e.g. `/sessions`).
    ///   - query: Optional query items.
    /// - Returns: The fully-qualified `URL`.
    public static func apiURL(_ path: String, query: [URLQueryItem] = []) throws -> URL {
        try resolveURL(prefix: apiPrefix, path: path, query: query)
    }

    /// Builds the health-check URL (`<server>/health`).
    public static func healthURL() throws -> URL {
        try resolveURL(prefix: "", path: "/health", query: [])
    }

    /// Builds a full `URL` for an SSE endpoint (prefixed with `/api`).
    public static func sseURL(_ path: String) throws -> URL {
        try resolveURL(prefix: apiPrefix, path: path, query: [])
    }

    /// The base origin string from `ConsoleConfig` (no trailing slash).
    public static var base: String { ConsoleConfig.serverURL }

    // MARK: - Internal

    /// Joins the server origin, a path prefix, and a relative path into a URL,
    /// then applies query items. Throws `AppError.config` for malformed input.
    private static func resolveURL(
        prefix: String,
        path: String,
        query: [URLQueryItem]
    ) throws -> URL {
        // Normalize the path so we never produce a double slash at the join.
        let relativePath: String
        if path.hasPrefix("/") {
            relativePath = path
        } else {
            relativePath = "/" + path
        }

        let joined = "\(ConsoleConfig.serverURL)\(prefix)\(relativePath)"

        guard var components = URLComponents(string: joined) else {
            throw AppError.config("Invalid URL for path: \(path)")
        }

        if !query.isEmpty {
            components.queryItems = query
        }

        guard let url = components.url else {
            throw AppError.config("Invalid URL for path: \(path)")
        }

        return url
    }
}
