import Foundation

/// Mirrors `config.rs` — manages the backend server origin at runtime.
///
/// URL construction for specific endpoints lives in ``ConsoleURLs``; this enum
/// only holds the mutable base origin string.
public enum ConsoleConfig {
    private static let lock = NSLock()
    private static var _serverURL: String = "http://localhost:3000"

    /// The configured server origin (no trailing slash).
    public static var serverURL: String {
        lock.lock(); defer { lock.unlock() }
        return _serverURL
    }

    /// Updates the server origin, stripping any trailing slash.
    public static func setServerURL(_ url: String) {
        let normalized = url.hasSuffix("/") ? String(url.dropLast()) : url
        lock.lock(); _serverURL = normalized; lock.unlock()
    }
}
