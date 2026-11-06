//! Embeds the Windows executable icon.
//!
//! Windows takes the window, taskbar and Explorer icon from the executable's
//! own resource table. macOS gets it from the `.app` bundle instead, so there
//! is nothing to compile there. GPUI asks for resource ID 1 (see
//! `gpui_windows::platform::load_icon`), which `console.rc` provides; without
//! that resource the app shows the generic icon everywhere.
//!
//! The whole block is `cfg(windows)` — build scripts are compiled for the
//! *host*, and `embed-resource` is only a Windows build-dependency, so a macOS
//! build neither compiles nor links it.

fn main() {
    emit_git_sha();
    emit_build_number();

    #[cfg(windows)]
    {
        println!("cargo:rerun-if-changed=console.rc");
        println!("cargo:rerun-if-changed=../assets/Console.ico");

        // The icon is cosmetic, so a missing resource compiler is not fatal —
        // but a compiler that runs and fails should fail the build rather than
        // ship an iconless exe.
        if let Err(result) =
            embed_resource::compile("console.rc", embed_resource::NONE).manifest_optional()
        {
            panic!("failed to embed the Windows icon resource: {result}");
        }
    }
}

/// Records the commit the binary was built from. The app logs it at startup,
/// so a report from an installed copy can be matched to a revision without
/// guessing which build is running.
fn emit_git_sha() {
    println!("cargo:rerun-if-changed=build.rs");
    // Re-run when HEAD moves. On a branch HEAD points at a ref, so watch the
    // branch too; in CI's detached checkout HEAD itself changes.
    println!("cargo:rerun-if-changed=../../../.git/HEAD");
    println!("cargo:rerun-if-changed=../../../.git/refs/heads/main");

    let Ok(output) = std::process::Command::new("git")
        .args(["rev-parse", "--short=12", "HEAD"])
        .output()
    else {
        return;
    };
    if !output.status.success() {
        return;
    }
    let sha = String::from_utf8_lossy(&output.stdout).trim().to_string();
    if !sha.is_empty() {
        println!("cargo:rustc-env=CONSOLE_GIT_SHA={sha}");
    }
}

/// CI's run number, which the in-app updater compares with the published
/// `latest.json`. Unset for local builds, which then skip update checks.
fn emit_build_number() {
    println!("cargo:rerun-if-env-changed=CONSOLE_BUILD_NUMBER");
    if let Ok(number) = std::env::var("CONSOLE_BUILD_NUMBER") {
        println!("cargo:rustc-env=CONSOLE_BUILD_NUMBER={number}");
    }
}
