use anyhow::anyhow;
use gpui::{App, AssetSource, Result, SharedString};
use std::borrow::Cow;

/// Native asset source — embeds `../assets` (relative to `src/` crate root).
#[derive(rust_embed::RustEmbed)]
#[folder = "../assets"]
#[include = "icons/**/*.svg"]
#[include = "images/**/*.png"]
#[include = "images/**/*.jpg"]
pub struct Assets;

impl AssetSource for Assets {
    fn load(&self, path: &str) -> Result<Option<Cow<'static, [u8]>>> {
        if path.is_empty() {
            return Ok(None);
        }

        Self::get(path)
            .map(|f| Some(f.data))
            .ok_or_else(|| anyhow!("could not find asset at path \"{}\"", path))
    }

    fn list(&self, path: &str) -> Result<Vec<SharedString>> {
        Ok(Self::iter()
            .filter_map(|p| p.starts_with(path).then(|| p.into()))
            .collect())
    }
}

/// Embedded UI fonts — Geist and Geist Mono (variable), © Vercel Inc.,
/// licensed under the SIL Open Font License 1.1.
static FONT_GEIST: &[u8] = include_bytes!("../assets/fonts/Geist.ttf");
static FONT_GEIST_MONO: &[u8] = include_bytes!("../assets/fonts/GeistMono.ttf");
static FONT_GEIST_MEDIUM: &[u8] = include_bytes!("../assets/fonts/Geist-Medium.ttf");
static FONT_GEIST_SEMIBOLD: &[u8] = include_bytes!("../assets/fonts/Geist-SemiBold.ttf");
static FONT_GEIST_BOLD: &[u8] = include_bytes!("../assets/fonts/Geist-Bold.ttf");

/// Register embedded Geist & Geist Mono fonts with GPUI
pub fn register_fonts(cx: &App) -> gpui::Result<()> {
    cx.text_system().add_fonts(vec![
        Cow::Borrowed(FONT_GEIST),
        Cow::Borrowed(FONT_GEIST_MONO),
        Cow::Borrowed(FONT_GEIST_MEDIUM),
        Cow::Borrowed(FONT_GEIST_SEMIBOLD),
        Cow::Borrowed(FONT_GEIST_BOLD),
    ])
}
