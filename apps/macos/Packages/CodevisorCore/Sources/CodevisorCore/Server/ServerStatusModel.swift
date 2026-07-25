import Combine
import Foundation

@MainActor
public final class ServerStatusModel: ObservableObject {
    @Published public private(set) var health: ServerHealth? = nil
    @Published public private(set) var info: ServerInfo? = nil
    @Published public private(set) var update: ServerUpdateInfo? = nil
    @Published public private(set) var errorMessage: String? = nil
    @Published public private(set) var isRefreshing = false

    private let client: any CodevisorServerClienting

    public init(client: any CodevisorServerClienting) {
        self.client = client
    }

    public func refresh() async {
        isRefreshing = true
        defer { isRefreshing = false }

        do {
            health = try await client.health()
            info = try await client.info()
            update = try await client.updateInfo()
            errorMessage = nil
        } catch {
            errorMessage = String(describing: error)
        }
    }

    public func issuePairingToken() async throws -> ServerPairingToken {
        try await client.issuePairingToken()
    }
}
