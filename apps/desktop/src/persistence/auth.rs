use std::fs;
use std::path::PathBuf;

fn path() -> PathBuf {
    super::store::storage_directory().join("server-auth.json")
}

#[derive(Clone, Debug, serde::Serialize, serde::Deserialize, Default)]
pub struct StoredServerAuth {
    pub server_url: String,
    pub token: String,
    pub device_id: String,
    pub device_name: String,
    pub expires_at: i64,
}

pub fn load() -> Option<StoredServerAuth> {
    let bytes = fs::read(path()).ok()?;
    serde_json::from_slice(&bytes).ok()
}

pub fn save(value: &StoredServerAuth) -> std::io::Result<()> {
    let dir = path().parent().unwrap();
    fs::create_dir_all(dir)?;
    // This file is deliberately mode 0600 and written atomically. The desktop
    // core keeps the token in memory; callers should prefer the platform
    // credential store when one is available and use this only as a local
    // fallback for headless/Linux installations.
    let tmp = path().with_extension(format!("json.{}.tmp", std::process::id()));
    let bytes = serde_json::to_vec(value).map_err(std::io::Error::other)?;
    fs::write(&tmp, bytes)?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&tmp, fs::Permissions::from_mode(0o600))?;
    }
    fs::rename(tmp, path())
}

pub fn clear() -> std::io::Result<()> {
    match fs::remove_file(path()) { Ok(()) | Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()), Err(e) => Err(e) }
}
