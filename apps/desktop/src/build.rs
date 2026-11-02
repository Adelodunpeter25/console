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
