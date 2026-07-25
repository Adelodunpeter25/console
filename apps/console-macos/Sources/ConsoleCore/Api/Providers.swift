import Foundation

// Mirrors api/providers.rs

public extension ApiClient {
    func listProviders() async throws -> [ProviderCatalogEntry] {
        try await get("/providers")
    }

    func getModels(providerId: String) async throws -> JSONValue {
        try await get("/providers/\(providerId)/models")
    }
}
