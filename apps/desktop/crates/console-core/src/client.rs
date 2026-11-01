use crate::services::*;
use crate::utils::HttpTransport;

#[derive(Clone)]
pub struct ConsoleClient {
    transport: HttpTransport,
    pub sessions: SessionService,
    pub runs: RunService,
    pub providers: ProviderService,
    pub assist: AssistService,
    pub devices: DeviceService,
    pub fs: FsService,
    pub projects: ProjectService,
    pub git: GitService,
    pub integrations: IntegrationService,
    pub auth: AuthService,
    pub server_auth: ServerAuthService,
    pub model_favorites: ModelFavoriteService,
    pub notifications: NotificationService,
    pub usage: UsageService,
    pub settings: SettingsService,
    pub ports: PortService,
    pub local_forwards: LocalForwardManager,
    pub scripts: ProjectScriptsService,
    pub mcp: McpService,
}

impl ConsoleClient {
    pub fn new(base_url: Option<String>) -> Self {
        Self::new_with_auth(base_url, None)
    }

    /// Build a client that already carries a stored device token, so every
    /// service is authenticated from the first request. Callers that pair at
    /// runtime use [`Self::set_auth_token`].
    pub fn new_with_auth(base_url: Option<String>, auth_token: Option<String>) -> Self {
        let transport = HttpTransport::new_with_auth(base_url, auth_token);
        Self {
            sessions: SessionService::new(transport.clone()),
            runs: RunService::new(transport.clone()),
            providers: ProviderService::new(transport.clone()),
            assist: AssistService::new(transport.clone()),
            devices: DeviceService::new(transport.clone()),
            fs: FsService::new(transport.clone()),
            projects: ProjectService::new(transport.clone()),
            git: GitService::new(transport.clone()),
            integrations: IntegrationService::new(transport.clone()),
            auth: AuthService::new(transport.clone()),
            server_auth: ServerAuthService::new(transport.clone()),
            model_favorites: ModelFavoriteService::new(transport.clone()),
            notifications: NotificationService::new(transport.clone()),
            usage: UsageService::new(transport.clone()),
            settings: SettingsService::new(transport.clone()),
            ports: PortService::new(transport.clone()),
            local_forwards: LocalForwardManager::new(transport.clone()),
            scripts: ProjectScriptsService::new(transport.clone()),
            mcp: McpService::new(transport.clone()),
            transport: transport.clone(),
        }
    }

    pub fn terminal_service(&self) -> TerminalService {
        TerminalService::new(self.transport.clone())
    }

    /// The shared HTTP transport, for services that need it without owning
    /// the client.
    pub fn transport(&self) -> &HttpTransport {
        &self.transport
    }

    pub async fn set_base_url(&self, url: impl Into<String>) {
        self.transport.set_base_url(url).await;
    }

    pub async fn base_url(&self) -> String {
        self.transport.base_url().await
    }

    pub async fn set_auth_token(&self, token: Option<String>) {
        self.transport.set_auth_token(token).await;
    }
}
