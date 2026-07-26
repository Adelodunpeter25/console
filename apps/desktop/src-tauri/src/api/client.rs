use reqwest::Client;

use once_cell::sync::Lazy;

use crate::config::api_base;
use crate::error::{AppError, AppResult};

/// A single shared `reqwest::Client` reused across every command so connection
/// pools are kept warm instead of being rebuilt per invocation.
static SHARED_HTTP: Lazy<Client> = Lazy::new(|| {
    Client::builder()
        .build()
        .expect("failed to build reqwest client")
});

pub fn shared_http_client() -> &'static Client {
    &SHARED_HTTP
}

#[derive(Clone)]
pub struct ApiClient {
    http: Client,
}

impl ApiClient {
    pub fn new() -> Self {
        Self {
            http: SHARED_HTTP.clone(),
        }
    }

    fn endpoint(path: &str) -> String {
        format!("{}{}", api_base(), path)
    }

    pub async fn get<T: serde::de::DeserializeOwned>(&self, path: &str) -> AppResult<T> {
        let url = Self::endpoint(path);
        let resp = self.http.get(&url).send().await?;
        Self::parse_response(resp).await
    }

    pub async fn get_with_query<T: serde::de::DeserializeOwned>(
        &self,
        path: &str,
        query: &[(&str, &str)],
    ) -> AppResult<T> {
        let url = Self::endpoint(path);
        let resp = self.http.get(&url).query(query).send().await?;
        Self::parse_response(resp).await
    }

    pub async fn post<T: serde::de::DeserializeOwned, B: serde::Serialize>(
        &self,
        path: &str,
        body: &B,
    ) -> AppResult<T> {
        let url = Self::endpoint(path);
        let resp = self.http.post(&url).json(body).send().await?;
        Self::parse_response(resp).await
    }

    pub async fn patch<T: serde::de::DeserializeOwned, B: serde::Serialize>(
        &self,
        path: &str,
        body: &B,
    ) -> AppResult<T> {
        let url = Self::endpoint(path);
        let resp = self.http.patch(&url).json(body).send().await?;
        Self::parse_response(resp).await
    }

    pub async fn delete<T: serde::de::DeserializeOwned>(&self, path: &str) -> AppResult<T> {
        let url = Self::endpoint(path);
        let resp = self.http.delete(&url).send().await?;
        Self::parse_response(resp).await
    }

    pub async fn delete_with_query<T: serde::de::DeserializeOwned>(
        &self,
        path: &str,
        query: &[(&str, &str)],
    ) -> AppResult<T> {
        let url = Self::endpoint(path);
        let resp = self.http.delete(&url).query(query).send().await?;
        Self::parse_response(resp).await
    }

    async fn parse_response<T: serde::de::DeserializeOwned>(
        resp: reqwest::Response,
    ) -> AppResult<T> {
        let status = resp.status();
        let text = resp.text().await?;

        if !status.is_success() {
            return Err(AppError::Server(format!("HTTP {}: {}", status, text)));
        }

        let parsed: crate::models::ApiResponse<T> = serde_json::from_str(&text)?;
        if !parsed.success {
            return Err(AppError::Api(
                parsed.error.unwrap_or_else(|| "Unknown API error".to_string()),
            ));
        }

        parsed
            .data
            .ok_or_else(|| AppError::Api("Response contained no data".to_string()))
    }

    pub fn raw_client(&self) -> &Client {
        &self.http
    }

    pub fn base_url(&self) -> String {
        api_base()
    }
}

impl Default for ApiClient {
    fn default() -> Self {
        Self::new()
    }
}
