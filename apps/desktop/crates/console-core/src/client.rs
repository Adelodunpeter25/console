use crate::services::*;
use crate::utils::HttpTransport;

#[derive(Clone)]
pub struct ConsoleClient {
    transport: HttpTransport,
    pub sessions: SessionService,
    pub runs: RunService,
    pub providers: ProviderService,
    pub assist: AssistService,
    pub fs: FsService,
    pub projects: ProjectService,
    pub git: GitService,
    pub model_favorites: ModelFavoriteService,
}

impl ConsoleClient {
    pub fn new(base_url: Option<String>) -> Self {
        let transport = HttpTransport::new(base_url);
        Self {
            sessions: SessionService::new(transport.clone()),
            runs: RunService::new(transport.clone()),
            providers: ProviderService::new(transport.clone()),
            assist: AssistService::new(transport.clone()),
            fs: FsService::new(transport.clone()),
            projects: ProjectService::new(transport.clone()),
            git: GitService::new(transport.clone()),
            model_favorites: ModelFavoriteService::new(transport.clone()),
            transport: transport.clone(),
        }
    }

    pub fn terminal_service(&self) -> TerminalService {
        TerminalService::new(self.transport.clone())
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
