use crate::net::WtTls;
use crate::services::*;
use crate::utils::HttpTransport;
use anyhow::Result;

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
        let transport = HttpTransport::new(base_url);
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

    /// The shared HTTP transport, for services that need it without
    /// building a `TerminalService` (the WT transport switch).
    pub fn transport(&self) -> &HttpTransport {
        &self.transport
    }

    /// Open a WebTransport session: mints the handshake token, then dials the
    /// QUIC listener (QUIC migration). Held by the caller for as long as its
    /// streams live — dropping it closes the connection. `origin` overrides
    /// the API origin for servers that listen separately, as
    /// `make dev-server-wt` does.
    pub async fn connect_wt(&self, origin: Option<&str>, tls: WtTls) -> Result<WtService> {
        WtService::connect(&self.transport, origin, tls).await
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
