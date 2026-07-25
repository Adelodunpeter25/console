import Foundation
import SwiftUI

/// The top-level view model for the entire app. Replaces the old
/// `AppEnvironment` — owns the `ApiClient`, auth state, project list, and
/// session list. All HTTP calls go through this.
@MainActor
public final class AppViewModel: ObservableObject {

    public let client: ApiClient

    // MARK: - Published state

    @Published public var authStatus: AuthStatusResponse?
    @Published public var projects: [ProjectInfo] = []
    @Published public var sessions: [SessionHeader] = []
    @Published public var providers: [ProviderCatalogEntry] = []
    @Published public var isLoading = false
    @Published public var errorMessage: String?
    @Published public var serverURL: String {
        didSet { ConsoleConfig.setServerURL(serverURL) }
    }

    // MARK: - Init

    public init(serverURL: String = "http://localhost:3000") {
        self.serverURL = serverURL
        ConsoleConfig.setServerURL(serverURL)
        self.client = ApiClient()
    }

    // MARK: - Health check

    public func pingServer() async -> Bool {
        guard let url = try? ConsoleURLs.healthURL() else { return false }
        do {
            let (_, resp) = try await URLSession.shared.data(from: url)
            return (resp as? HTTPURLResponse)?.statusCode == 200
        } catch {
            return false
        }
    }

    // MARK: - Auth

    public func refreshAuthStatus() async {
        do {
            authStatus = try await client.getAuthStatus()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    public func login(provider: String) async {
        do {
            _ = try await client.getLoginURL(provider: provider)
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    // MARK: - Projects

    public func refreshProjects() async {
        do {
            projects = try await client.listProjects()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    public func addProject(path: String) async {
        do {
            let project = try await client.addProject(path: path)
            projects.append(project)
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    // MARK: - Sessions

    public func refreshSessions(cwd: String? = nil, projectId: String? = nil) async {
        do {
            sessions = try await client.listSessions(cwd: cwd, projectId: projectId)
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    public func createSession(cwd: String, projectId: String? = nil, modelId: String? = nil, provider: String? = nil, title: String? = nil) async -> SessionHeader? {
        do {
            let dto = CreateSessionDto(cwd: cwd, projectId: projectId, modelId: modelId, provider: provider, title: title)
            let session = try await client.createSession(dto)
            sessions.insert(session, at: 0)
            return session
        } catch {
            errorMessage = error.localizedDescription
            return nil
        }
    }

    public func deleteSession(id: String) async {
        do {
            _ = try await client.deleteSession(id: id)
            sessions.removeAll { $0.id == id }
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    // MARK: - Providers

    public func refreshProviders() async {
        do {
            providers = try await client.listProviders()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    // MARK: - Initial load

    public func initialLoad() async {
        isLoading = true
        defer { isLoading = false }

        await refreshAuthStatus()
        await refreshProjects()
        await refreshSessions()
        await refreshProviders()
    }
}
