//! In-app updates for the desktop build. CI publishes `latest.json` next to the
//! platform zips on the rolling `console-desktop` release; the app compares its
//! own build number with it, downloads the zip for this platform and swaps
//! itself out after it quits.

use anyhow::{Context, Result, anyhow};
use futures_util::StreamExt;
use serde::Deserialize;
use std::path::{Path, PathBuf};

const RELEASE_BASE: &str =
    "https://github.com/Adelodunpeter25/console/releases/download/console-desktop";

#[derive(Clone, Debug, Deserialize, Eq, PartialEq)]
pub struct UpdateManifest {
    pub build: u64,
}

pub fn parse_manifest(text: &str) -> Result<UpdateManifest> {
    serde_json::from_str(text).context("invalid latest.json")
}

impl UpdateManifest {
    pub fn is_newer_than(&self, installed_build: u64) -> bool {
        self.build > installed_build
    }
}

/// Release asset holding this platform's build, or None where there is none.
pub fn asset_name(os: &str, arch: &str) -> Option<&'static str> {
    match (os, arch) {
        ("macos", "aarch64") => Some("Console-macos-arm64.zip"),
        ("macos", "x86_64") => Some("Console-macos-x64.zip"),
        ("windows", "x86_64") => Some("Console-windows-x64.zip"),
        _ => None,
    }
}

fn current_asset() -> Result<&'static str> {
    asset_name(std::env::consts::OS, std::env::consts::ARCH)
        .ok_or_else(|| anyhow!("in-app updates are not available on this platform"))
}

/// The published manifest when it is newer than `installed_build`.
pub async fn check(installed_build: u64) -> Result<Option<UpdateManifest>> {
    current_asset()?;
    let response = reqwest::get(format!("{RELEASE_BASE}/latest.json"))
        .await?
        .error_for_status()
        .context("update check failed")?;
    let manifest = parse_manifest(&response.text().await?)?;
    Ok(manifest.is_newer_than(installed_build).then_some(manifest))
}

/// Downloads this platform's zip into the temp dir, reporting 0..1 progress.
pub async fn download(mut on_progress: impl FnMut(f32)) -> Result<PathBuf> {
    let asset = current_asset()?;
    let response = reqwest::get(format!("{RELEASE_BASE}/{asset}"))
        .await?
        .error_for_status()
        .context("download failed")?;
    let total = response.content_length().unwrap_or(0);
    let dest = std::env::temp_dir().join(asset);
    let mut file = tokio::fs::File::create(&dest).await?;
    let mut stream = response.bytes_stream();
    let mut read = 0u64;
    while let Some(chunk) = stream.next().await {
        let chunk = chunk?;
        tokio::io::AsyncWriteExt::write_all(&mut file, &chunk).await?;
        read += chunk.len() as u64;
        if total > 0 {
            on_progress(read as f32 / total as f32);
        }
    }
    Ok(dest)
}

fn unpack_dir() -> Result<PathBuf> {
    let dir = std::env::temp_dir().join("console-update");
    let _ = std::fs::remove_dir_all(&dir);
    std::fs::create_dir_all(&dir)?;
    Ok(dir)
}

fn run(command: &mut std::process::Command) -> Result<()> {
    let status = command.status()?;
    if !status.success() {
        return Err(anyhow!("{command:?} exited with {status}"));
    }
    Ok(())
}

/// Unpacks the zip and starts a detached helper that waits for this process to
/// exit, swaps the new build in and relaunches it. The caller must quit right
/// after this returns Ok.
#[cfg(target_os = "macos")]
pub fn install_and_relaunch(zip: &Path) -> Result<()> {
    let exe = std::env::current_exe()?;
    // .../Console.app/Contents/MacOS/console
    let bundle = exe
        .ancestors()
        .find(|p| p.extension().is_some_and(|e| e == "app"))
        .ok_or_else(|| anyhow!("not running from an app bundle"))?
        .to_path_buf();
    let dir = unpack_dir()?;
    run(std::process::Command::new("ditto")
        .args(["-x", "-k"])
        .arg(zip)
        .arg(&dir))?;
    let fresh = std::fs::read_dir(&dir)?
        .filter_map(|e| e.ok().map(|e| e.path()))
        .find(|p| p.extension().is_some_and(|e| e == "app"))
        .ok_or_else(|| anyhow!("the update did not contain an app bundle"))?;
    std::process::Command::new("sh")
        .arg("-c")
        .arg(
            r#"while kill -0 "$3" 2>/dev/null; do sleep 0.2; done; rm -rf "$1" && mv "$2" "$1" && open "$1""#,
        )
        .arg("sh")
        .arg(&bundle)
        .arg(&fresh)
        .arg(std::process::id().to_string())
        .spawn()?;
    Ok(())
}

#[cfg(target_os = "windows")]
pub fn install_and_relaunch(zip: &Path) -> Result<()> {
    let exe = std::env::current_exe()?;
    let dir = unpack_dir()?;
    run(std::process::Command::new("powershell")
        .args([
            "-NoProfile",
            "-Command",
            "Expand-Archive -Force -LiteralPath $args[0] -DestinationPath $args[1]",
        ])
        .arg(zip)
        .arg(&dir))?;
    let installer = dir.join("Console-Setup-x64.exe");
    // The installer overwrites console.exe, so it has to wait for us to exit;
    // it reuses the existing install directory from the registry.
    std::process::Command::new("cmd")
        .arg("/C")
        .arg(format!(
            "timeout /t 2 /nobreak >nul & \"{}\" /S & start \"\" \"{}\"",
            installer.display(),
            exe.display()
        ))
        .spawn()?;
    Ok(())
}

#[cfg(not(any(target_os = "macos", target_os = "windows")))]
pub fn install_and_relaunch(_zip: &Path) -> Result<()> {
    Err(anyhow!("in-app updates are not available on this platform"))
}
