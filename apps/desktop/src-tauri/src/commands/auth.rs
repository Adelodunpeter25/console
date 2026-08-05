use crate::api::ApiClient;
use crate::error::{AppError, AppResult};
use crate::models::{AuthStatusResponse, OAuthCallbackDto, OAuthLoginUrlDto};
use serde::{Deserialize, Serialize};
use std::net::TcpListener;
use tauri_plugin_opener::OpenerExt;

#[derive(Debug, Serialize, Deserialize)]
struct LoginUrlResponse {
    #[serde(rename = "authUrl")]
    auth_url: String,
    #[serde(rename = "redirectUri")]
    redirect_uri: String,
}

#[derive(Debug, Serialize, Deserialize)]
struct CallbackResult {
    provider: String,
    #[serde(rename = "userEmail", skip_serializing_if = "Option::is_none")]
    user_email: Option<String>,
}

/// Parse the port and path from a redirect URI like `http://localhost:8085/oauth2callback`.
fn parse_redirect_uri(uri: &str) -> Result<(u16, String), AppError> {
    // Expected format: http://localhost:PORT/PATH or http://127.0.0.1:PORT/PATH
    let after_scheme = uri.strip_prefix("http://").or_else(|| uri.strip_prefix("https://"))
        .ok_or_else(|| AppError::Other(format!("Redirect URI missing http(s):// scheme: {uri}")))?;

    // Split host:port from path at the first '/'.
    let (host_port, path) = match after_scheme.find('/') {
        Some(idx) => (&after_scheme[..idx], &after_scheme[idx..]),
        None => (after_scheme, "/"),
    };

    // Extract the port from host:port (after the last colon).
    let port = host_port.rsplit(':').next()
        .ok_or_else(|| AppError::Other(format!("Missing port in redirect URI host: {host_port}")))?
        .parse::<u16>()
        .map_err(|e| AppError::Other(format!("Invalid port in redirect URI: {e}")))?;

    Ok((port, path.to_string()))
}

/// Start a local HTTP server on the OAuth callback port, wait for the
/// redirect, extract the `code` and `state` query params, and return them.
fn wait_for_callback(port: u16, callback_path: &str) -> Result<(String, Option<String>), AppError> {
    let listener = TcpListener::bind(format!("127.0.0.1:{}", port))
        .map_err(|e| AppError::Other(format!("Failed to bind callback server on port {port}: {e}. Is another login already in progress?")))?;

    // Accept the first connection that hits the callback path.
    let (mut stream, _) = listener.accept()
        .map_err(|e| AppError::Other(format!("Failed to accept callback connection: {e}")))?;

    use std::io::{Read, Write};

    // Read the HTTP request.
    let mut buf = [0u8; 4096];
    let n = stream.read(&mut buf)
        .map_err(|e| AppError::Other(format!("Failed to read callback request: {e}")))?;

    let request = String::from_utf8_lossy(&buf[..n]);

    // Parse the first line: GET /path?code=...&state=... HTTP/1.1
    let request_line = request.lines().next()
        .ok_or_else(|| AppError::Other("Empty callback request".into()))?;

    // Extract the path + query from the request line.
    let parts: Vec<&str> = request_line.split_whitespace().collect();
    let path_with_query = parts.get(1)
        .ok_or_else(|| AppError::Other("Malformed callback request line".into()))?;

    // Verify it's hitting the expected callback path.
    if !path_with_query.starts_with(callback_path) {
        return Err(AppError::Other(format!(
            "Unexpected callback path: expected {callback_path}, got {path_with_query}"
        )));
    }

    // Parse query parameters.
    let query = path_with_query.split('?').nth(1).unwrap_or("");
    let mut code: Option<String> = None;
    let mut state: Option<String> = None;

    for pair in query.split('&') {
        let mut kv = pair.splitn(2, '=');
        let key = kv.next().unwrap_or("");
        let value = kv.next().unwrap_or("");
        match key {
            "code" => code = Some(url_decode(value)),
            "state" => state = Some(url_decode(value)),
            _ => {}
        }
    }

    // Send a success response to the browser so the user sees confirmation.
    let response = "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nConnection: close\r\n\r\n\
        <!DOCTYPE html><html><head><title>Login Successful</title></head>\
        <body><h1>Successfully authenticated!</h1>\
        <p>You can close this tab and return to Console.</p>\
        </body></html>";
    let _ = stream.write_all(response.as_bytes());
    let _ = stream.flush();

    let code = code.ok_or_else(|| AppError::Other("Callback missing 'code' parameter".into()))?;
    Ok((code, state))
}

/// Simple URL-decode for percent-encoded query values.
fn url_decode(s: &str) -> String {
    let mut result = String::with_capacity(s.len());
    let mut chars = s.chars().peekable();
    while let Some(c) = chars.next() {
        if c == '%' {
            let hex: String = chars.by_ref().take(2).collect();
            if let Ok(byte) = u8::from_str_radix(&hex, 16) {
                result.push(byte as char);
            }
        } else if c == '+' {
            result.push(' ');
        } else {
            result.push(c);
        }
    }
    result
}

#[tauri::command]
pub async fn get_auth_status() -> AppResult<AuthStatusResponse> {
    let client = ApiClient::new();
    crate::api::auth::get_auth_status(&client).await
}

#[tauri::command]
pub async fn get_login_url(provider: String) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    let dto = OAuthLoginUrlDto { provider };
    crate::api::auth::get_login_url(&client, &dto).await
}

#[tauri::command]
pub async fn handle_oauth_callback(provider: String, code: String, state: Option<String>) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    let dto = OAuthCallbackDto { provider, code, state };
    crate::api::auth::handle_callback(&client, &dto).await
}

#[tauri::command]
pub async fn get_project_id(provider: String) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    crate::api::auth::get_project_id(&client, &provider).await
}

#[tauri::command]
pub async fn set_project_id(provider: String, project_id: Option<String>) -> AppResult<serde_json::Value> {
    let client = ApiClient::new();
    let dto = crate::api::auth::ProjectIdDto { provider, project_id };
    crate::api::auth::set_project_id(&client, &dto).await
}

/// Full automatic OAuth login flow:
/// 1. Get the login URL from the backend
/// 2. Start a local callback server on the OAuth port
/// 3. Open the auth URL in the system browser
/// 4. Wait for the redirect, extract the code
/// 5. Submit the code to the backend for token exchange
/// 6. Return the result (email, projectId)
#[tauri::command]
pub async fn login_with_browser(
    app: tauri::AppHandle,
    provider: String,
) -> AppResult<serde_json::Value> {
    // 1. Get the login URL from the backend.
    let client = ApiClient::new();
    let dto = OAuthLoginUrlDto { provider: provider.clone() };
    let login_url_value = crate::api::auth::get_login_url(&client, &dto).await?;
    let login_url_resp: LoginUrlResponse = serde_json::from_value(login_url_value)
        .map_err(|e| AppError::Other(format!("Failed to parse login URL response: {e}")))?;

    // 2. Parse the redirect URI to get the port and callback path.
    let (port, callback_path) = parse_redirect_uri(&login_url_resp.redirect_uri)?;

    // 3. Open the browser. We do this in parallel with starting the callback
    //    server so the server is ready when the redirect comes.
    let auth_url = login_url_resp.auth_url.clone();
    let app_handle = app.clone();
    // Open browser on the main thread.
    let _ = app_handle.opener().open_url(auth_url, None::<&str>);

    // 4. Wait for the callback in a blocking thread (TcpListener is blocking).
    let provider_clone = provider.clone();
    let (code, state) = tokio::task::spawn_blocking(move || {
        wait_for_callback(port, &callback_path)
    })
    .await
    .map_err(|e| AppError::Other(format!("Callback task failed: {e}")))??;

    // 5. Submit the code to the backend for token exchange.
    let callback_dto = OAuthCallbackDto {
        provider: provider_clone,
        code,
        state,
    };
    let result: serde_json::Value = crate::api::auth::handle_callback(&client, &callback_dto).await?;

    // 6. Return the result.
    Ok(result)
}
