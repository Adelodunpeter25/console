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
    pub fn load_providers(&mut self, cx: &mut gpui::Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            if let Ok(providers) = client.providers.list().await {
                cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.providers = Rc::from(providers);
                            cx.notify();
                        });
                    }
                });
            }
        })
        .detach();
    }

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
                        match result {
                            // An empty list means "not logged in yet" or a
                            // failed discovery, not a real catalog — caching
                            // it would hide the models a login moments later
                            // makes available.
                            Ok(models) if !models.is_empty() => {
                                Rc::make_mut(&mut this.models_by_provider)
                                    .insert(provider_id, models);
                            }
                            _ => {}
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

    /// Drops one provider's cached live model list so the next picker open
    /// refetches it. Called after credential changes (OAuth login, account
    /// switch, account removal): the server's discovery cache revalidates,
    /// and a stale client copy would keep serving the old account's lineup.
    pub(crate) fn invalidate_provider_models(&mut self, provider_id: &str) {
        Rc::make_mut(&mut self.models_by_provider).remove(provider_id);
        self.loading_models.remove(provider_id);
    }

    pub fn toggle_model_favorite(
        &mut self,
        provider: String,
        model_id: String,
        cx: &mut gpui::Context<Self>,
    ) {
        let key = format!("{}:{}", provider, model_id);
        let is_favorite = if self.favorites.contains(&key) {
            Rc::make_mut(&mut self.favorites).remove(&key);
            false
        } else {
            Rc::make_mut(&mut self.favorites).insert(key);
            true
        };
        let client = self.client.clone();
        let entity = cx.entity().downgrade();
        cx.spawn(async move |_entity, cx| {
            if let Err(error) = client
                .model_favorites
                .set(
                    console_core::ModelFavorite { provider, model_id },
                    is_favorite,
                )
                .await
            {
                let message = format!("Unable to update model favorite: {error}");
                let _ = cx.update(|cx| {
                    if let Some(app) = entity.upgrade() {
                        app.update(cx, |this, cx| {
                            this.set_error(message, cx);
                        });
                    }
                });
            }
        })
        .detach();
        cx.notify();
    }
}
