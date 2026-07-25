import Foundation

/// Mirrors `error.rs` — all error cases the API client can produce.
public enum AppError: Error, LocalizedError, Sendable {
    case http(String)
    case json(String)
    case server(String)
    case api(String)
    case config(String)
    case sse(String)

    public var errorDescription: String? {
        switch self {
        case .http(let m):   return "HTTP request failed: \(m)"
        case .json(let m):   return "Failed to parse response: \(m)"
        case .server(let m): return "Server returned error: \(m)"
        case .api(let m):    return "API returned unsuccessful response: \(m)"
        case .config(let m): return "Configuration error: \(m)"
        case .sse(let m):    return "SSE stream error: \(m)"
        }
    }
}
