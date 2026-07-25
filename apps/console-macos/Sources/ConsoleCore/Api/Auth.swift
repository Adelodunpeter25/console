import Foundation

// Mirrors api/auth.rs

public extension ApiClient {
    func getAuthStatus() async throws -> AuthStatusResponse {
        try await get("/auth/status")
    }

    func getLoginURL(provider: String) async throws -> JSONValue {
        let body = OAuthLoginUrlDto(provider: provider)
        return try await post("/auth/login/url", body: body)
    }

    func handleCallback(provider: String, code: String, state: String? = nil) async throws -> JSONValue {
        let body = OAuthCallbackDto(provider: provider, code: code, state: state)
        return try await post("/auth/login/callback", body: body)
    }
}
