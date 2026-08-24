//! Dynamic model discovery — fetches each provider's live model list from
//! `/api/providers/:id/models` and caches it on [`ConsoleDesktopApp`].
//!
//! The static `models` embedded in `ProviderCatalogEntry` (returned by
//! `/api/providers`) are always available as a fallback, so the picker is never
//! blank: it shows static models immediately and swaps in the live list when
//! the fetch lands. This mirrors the Electron and mobile pickers, which use
//! `modelsByProvider[provider.name] ?? provider.models`.

use std::rc::Rc;

use super::app::ConsoleDesktopApp;

impl ConsoleDesktopApp {
    /// Fire a background fetch for a provider's live model list, unless one is
    /// already cached or in flight. Idempotent — safe to call on every render
    /// and every tab switch. On success the result replaces the cache entry and
    /// the app re-renders; on failure the loading flag is cleared and the
    /// static fallback remains, matching the server's own silent-fallback
    /// behaviour in `fetchModelsForProvider`.
    pub(crate) fn load_models_for_provider(
        &mut self,
        provider_id: &str,
        cx: &mut gpui::Context<Self>,
    ) {
        if self.models_by_provider.contains_key(provider_id)
            || self.loading_models.contains(provider_id)
        {
            return;
        }
        self.loading_models.insert(provider_id.to_string());
        let client = self.client.clone();
        let provider_id = provider_id.to_string();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            let result = client.providers.get_models(&provider_id).await;
            cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.loading_models.remove(&provider_id);
                        if let Ok(models) = result {
                            Rc::make_mut(&mut this.models_by_provider)
                                .insert(provider_id, models);
                        }
                        // On error the static fallback stays in place; no
                        // error banner — the picker is still usable.
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }
}
