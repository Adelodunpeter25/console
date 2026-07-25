import Foundation

/// Mirrors `config.rs` — manages the backend server URL at runtime.
public enum ConsoleConfig {
    private static let lock = NSLock()
    private static var _serverURL: String = "http://localhost:3000"

    public static var serverURL: String {
        lock.lock(); defer { lock.unlock() }
        return _serverURL
    }

    public static func setServerURL(_ url: String) {
        let normalized = url.hasSuffix("/") ? String(url.dropLast()) : url
        lock.lock(); _serverURL = normalized; lock.unlock()
    }

    public static var apiBase: String { "\(serverURL)/api" }
    public static var healthURL: String { "\(serverURL)/health" }
}
