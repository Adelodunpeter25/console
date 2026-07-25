import Foundation

// MARK: - ApiResponse<T>  (mirrors models/api.rs)
// Decodable-only: we never encode this — we only decode server responses.
public struct ApiResponse<T: Decodable & Sendable>: Decodable {
    public let success: Bool
    public let data: T?
    public let error: String?
}
