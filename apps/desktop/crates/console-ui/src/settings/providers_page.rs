use crate::input::ComposerInput;
use crate::settings::GitHubSection;
use crate::theme::Theme;
use console_core::types::{AuthStatusResponse, ProviderAccount, ProviderCatalogEntry, auth_status_for};
use gpui::prelude::FluentBuilder;
use gpui::{
    App, ElementId, Entity, InteractiveElement, IntoElement, MouseButton, ParentElement, RenderOnce,
    StatefulInteractiveElement, Styled, Window, div, px,
};
use std::collections::HashSet;
use std::rc::Rc;

#[derive(IntoElement)]
pub struct ProvidersPage {
    pub providers: Rc<Vec<ProviderCatalogEntry>>,
    pub auth_status: Option<AuthStatusResponse>,
    pub logging_in: HashSet<String>,
    pub on_login: Rc<dyn Fn(String, &mut Window, &mut App) + 'static>,
    /// (provider, account id) a Use or Remove click is mutating right now, so
    /// that row's buttons read in-flight instead of firing duplicates.
    pub account_busy: Option<(String, String)>,
    /// The account the most recent OAuth callback landed in, as
    /// (provider, id). Highlighted so a re-login that lands in a NEW account
    /// is visible immediately. Cleared when the next login starts.
    pub recent_login_account: Option<(String, String)>,
    pub on_account_use: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    pub on_account_remove: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    /// (provider, account id, current display name): opens the rename
    /// dialog. The dialog itself renders above the whole window (built by
    /// the settings window), like the server modals.
    pub on_account_rename:
        Rc<dyn Fn(String, String, String, &mut Window, &mut App) + 'static>,
    /// (provider, server-authored failure text) for the last account
    /// mutation failure, shown verbatim on that provider's card.
    pub account_error: Option<(String, String)>,
    /// GitHub git-credential state. Rendered below the LLM providers as a
    /// sibling section — GitHub is not a chat provider and authenticates by
    /// token, not OAuth.
    pub github: Option<GitHubSection>,
    /// Providers that authenticate by a pasted API key, in catalog order.
    /// Each carries its own form state and callbacks, keyed by provider id.
    pub api_keys: Vec<crate::settings::ApiKeyProviderSection>,
}

/// What an account row shows instead of the raw email: its label when the
/// user named it, else the local part of the email — readable, and the full
/// address never renders unless the user typed it as a name themselves.
fn account_display(account: &ProviderAccount) -> String {
    let label = account.label.clone().filter(|l| !l.is_empty());
    if let Some(label) = label {
        return label;
    }
    if let Some(email) = account.email.clone().filter(|e| !e.is_empty()) {
        if let Some(local) = email.split('@').next() {
            if !local.is_empty() {
                return local.to_string();
            }
        }
    }
    account.id.clone()
}

#[allow(clippy::too_many_arguments)]
fn account_row(
    theme: &Theme,
    provider: &str,
    account: &ProviderAccount,
    is_active: bool,
    highlighted: bool,
    busy: bool,
    on_use: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_remove: Rc<dyn Fn(String, String, &mut Window, &mut App) + 'static>,
    on_rename: Rc<dyn Fn(String, String, String, &mut Window, &mut App) + 'static>,
) -> gpui::AnyElement {
    let account_id = account.id.clone();
    let display = account_display(account);
    let is_env = account.env.unwrap_or(false);

    let mut row = div()
        .id(ElementId::from(format!("account-{provider}-{account_id}")))
        .flex()
        .items_center()
        .justify_between()
        .px(px(8.0))
        .py(px(5.0))
        .rounded(px(6.0))
        .bg(if highlighted {
            theme.overlay_strong
        } else {
            theme.inset
        })
        .gap(px(8.0))
        .child(
            div()
                .flex()
                .items_center()
                .gap(px(8.0))
                .child(
                    div()
                        .text_size(px(12.0))
                        .text_color(theme.text)
                        .child(display.clone()),
                )
                // The synthetic stand-in for an environment-token login (plan
                // §4.4): it always owns the provider while present, so a Use
                // button would lie.
                .children(is_env.then(|| {
                    div()
                        .text_size(px(10.0))
                        .text_color(theme.text_tertiary)
                        .child("Environment token")
                }))
                .children(is_active.then(|| {
                    div()
                        .text_size(px(10.0))
                        .font_weight(gpui::FontWeight::SEMIBOLD)
                        .text_color(theme.accent)
                        .px(px(6.0))
                        .py(px(1.0))
                        .rounded(px(4.0))
                        .bg(theme.overlay_strong)
                        .child("ACTIVE")
                })),
        );

    if !is_env {
        row = row.child(
            div()
                .flex()
                .items_center()
                .gap(px(10.0))
                .children((!is_active).then(|| {
                    let use_id = account_id.clone();
                    let on_use = on_use.clone();
                    let provider = provider.to_string();
                    div()
                        .id(ElementId::from(format!("use-{provider}-{use_id}")))
                        .text_size(px(11.0))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(if busy { theme.text_ghost } else { theme.text_secondary })
                        .cursor_pointer()
                        .hover(|s| s.text_color(theme.accent))
                        .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                            cx.stop_propagation();
                            (on_use)(provider.clone(), use_id.clone(), window, cx);
                        })
                        .child(if busy { "Using..." } else { "Use" })
                }))
                .child({
                    // Rename works on the active account too: labels are
                    // display-only, so they never interfere with pinning.
                    let rename_id = account_id.clone();
                    let rename_display = display.clone();
                    let on_rename = on_rename.clone();
                    let provider = provider.to_string();
                    div()
                        .id(ElementId::from(format!("rename-{provider}-{rename_id}")))
                        .text_size(px(11.0))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.text_secondary)
                        .cursor_pointer()
                        .hover(|s| s.text_color(theme.text))
                        .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                            cx.stop_propagation();
                            (on_rename)(
                                provider.clone(),
                                rename_id.clone(),
                                rename_display.clone(),
                                window,
                                cx,
                            );
                        })
                        .child("Rename")
                })
                .child({
                    let remove_id = account_id.clone();
                    let on_remove = on_remove.clone();
                    let provider = provider.to_string();
                    div()
                        .id(ElementId::from(format!("remove-{provider}-{remove_id}")))
                        .text_size(px(11.0))
                        .font_weight(gpui::FontWeight::MEDIUM)
                        .text_color(theme.text_secondary)
                        .cursor_pointer()
                        .hover(|s| s.text_color(theme.danger))
                        .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                            cx.stop_propagation();
                            (on_remove)(provider.clone(), remove_id.clone(), window, cx);
                        })
                        .child(if busy { "Removing..." } else { "Remove" })
                }),
        );
    }
    row.into_any_element()
}

impl RenderOnce for ProvidersPage {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_login = self.on_login.clone();

        div()
            .flex()
            .flex_col()
            .gap(px(16.0))
            .child(
                div()
                    .flex()
                    .flex_col()
                    .gap(px(4.0))
                    .child(
                        div()
                            .text_size(px(16.0))
                            .font_weight(gpui::FontWeight::SEMIBOLD)
                            .text_color(theme.text)
                            .child("AI Providers"),
                    )
                    .child(
                        div()
                            .text_size(px(12.5))
                            .text_color(theme.text_secondary)
                            .child(
                                "Manage provider credentials and authentication for LLM agents.",
                            ),
                    ),
            )
            .child(
                div().flex().flex_col().gap(px(10.0)).children(
                    self.providers
                        .iter()
                        .filter(|p| p.auth_method != "none" && p.auth_method != "api-key")
                        .map(|provider| {
                            let provider_id = provider.name.clone();
                            let is_logging_in = self.logging_in.contains(&provider_id);
                            let prov_status = self
                                .auth_status
                                .as_ref()
                                .and_then(|st| auth_status_for(st, &provider_id))
                                .cloned();

                            let is_logged_in =
                                prov_status.as_ref().map_or(false, |s| s.logged_in);
                            // "Connected as …" shows the account's display
                            // name (label or email local part), never the raw
                            // email.
                            let active_name = prov_status
                                .as_ref()
                                .and_then(|s| s.accounts.iter().find(|a| a.active.unwrap_or(false)))
                                .map(account_display);
                            let accounts = prov_status
                                .as_ref()
                                .map(|s| s.accounts.clone())
                                .unwrap_or_default();
                            let any_active = accounts.iter().any(|a| a.active.unwrap_or(false));
                            // Switched-provider id matching: the catalog may
                            // say "openai"/"anthropic" for codex/claude while
                            // status and callbacks use the canonical ids, but
                            // both pages key on the same strings the server
                            // sends, so plain equality is right here.
                            let recent = self
                                .recent_login_account
                                .as_ref()
                                .filter(|(p, _)| p == &provider_id)
                                .map(|(_, id)| id.clone());
                            let row_busy = self
                                .account_busy
                                .as_ref()
                                .filter(|(p, _)| p == &provider_id)
                                .map(|(_, id)| id.clone());
                            let account_error = self
                                .account_error
                                .as_ref()
                                .filter(|(p, _)| p == &provider_id)
                                .map(|(_, text)| text.clone());

                            let status_text = if is_logging_in {
                                "Authenticating...".to_string()
                            } else if is_logged_in {
                                if let Some(name) = active_name {
                                    format!("Connected as {name}")
                                } else {
                                    "Connected".to_string()
                                }
                            } else {
                                "Not connected".to_string()
                            };

                            let status_color = if is_logging_in || is_logged_in {
                                theme.accent
                            } else {
                                theme.text_ghost
                            };

                            let pid = provider_id.clone();
                            let on_login_click = on_login.clone();
                            let add_pid = provider_id.clone();
                            let on_add_click = on_login.clone();
                            let on_use = self.on_account_use.clone();
                            let on_remove = self.on_account_remove.clone();
                            let on_rename = self.on_account_rename.clone();

                            div()
                                .id(ElementId::from(format!("provider-row-{pid}")))
                                .p(px(12.0))
                                .rounded(px(8.0))
                                .border_1()
                                .border_color(theme.border)
                                .bg(theme.surface)
                                .flex()
                                .flex_col()
                                .gap(px(10.0))
                                .when_some(account_error, |card, error| {
                                    card.child(
                                        div()
                                            .text_size(px(11.5))
                                            .text_color(theme.danger)
                                            .child(error),
                                    )
                                })
                                .child(
                                    div()
                                        .flex()
                                        .items_center()
                                        .justify_between()
                                        .child(
                                            div()
                                                .flex()
                                                .items_center()
                                                .gap(px(10.0))
                                                .child(
                                                    div()
                                                        .w(px(8.0))
                                                        .h(px(8.0))
                                                        .rounded_full()
                                                        .bg(status_color),
                                                )
                                                .child(
                                                    div()
                                                        .flex()
                                                        .flex_col()
                                                        .gap(px(2.0))
                                                        .child(
                                                            div()
                                                                .text_size(px(13.5))
                                                                .font_weight(
                                                                    gpui::FontWeight::MEDIUM,
                                                                )
                                                                .text_color(theme.text)
                                                                .child(
                                                                    provider.display_name.clone(),
                                                                ),
                                                        )
                                                        .child(
                                                            div()
                                                                .text_size(px(11.5))
                                                                .text_color(theme.text_tertiary)
                                                                .child(status_text),
                                                        ),
                                                ),
                                        )
                                        .child(
                                            div()
                                                .flex()
                                                .items_center()
                                                .gap(px(10.0))
                                                // "+ Add account" uses the same
                                                // login flow: the callback
                                                // registers under the
                                                // provider's identity and
                                                // never steals active (§0).
                                                .child(
                                                    div()
                                                        .id(ElementId::from(format!(
                                                            "btn-add-{add_pid}"
                                                        )))
                                                        .text_size(px(12.0))
                                                        .font_weight(gpui::FontWeight::MEDIUM)
                                                        .text_color(theme.text_ghost)
                                                        .cursor_pointer()
                                                        .hover(|s| s.text_color(theme.text))
                                                        .on_mouse_down(
                                                            MouseButton::Left,
                                                            move |_event, window, cx| {
                                                                cx.stop_propagation();
                                                                (on_add_click)(
                                                                    add_pid.clone(),
                                                                    window,
                                                                    cx,
                                                                );
                                                            },
                                                        )
                                                        .child(if is_logging_in {
                                                            "Authenticating..."
                                                        } else {
                                                            "+ Add account"
                                                        }),
                                                )
                                                .child(
                                                    div()
                                                        .id(ElementId::from(format!(
                                                            "btn-login-{pid}"
                                                        )))
                                                        .px(px(10.0))
                                                        .py(px(5.0))
                                                        .rounded(px(6.0))
                                                        .border_1()
                                                        .border_color(theme.border_strong)
                                                        .bg(theme.raised)
                                                        .cursor_pointer()
                                                        .hover(|s| s.bg(theme.overlay))
                                                        .active(|s| s.bg(theme.overlay_strong))
                                                        .on_mouse_down(
                                                            MouseButton::Left,
                                                            move |_event, window, cx| {
                                                                cx.stop_propagation();
                                                                (on_login_click)(
                                                                    pid.clone(),
                                                                    window,
                                                                    cx,
                                                                );
                                                            },
                                                        )
                                                        .child(
                                                            div()
                                                                .text_size(px(12.0))
                                                                .font_weight(
                                                                    gpui::FontWeight::MEDIUM,
                                                                )
                                                                .text_color(theme.text)
                                                                .child(if is_logging_in {
                                                                    "Authenticating..."
                                                                } else if is_logged_in {
                                                                    "Re-login"
                                                                } else {
                                                                    "Login"
                                                                }),
                                                        ),
                                                ),
                                        ),
                                )
                                // Account list: every stored login for this
                                // provider, oldest first. A provider with
                                // rows but no ACTIVE account is disabled
                                // (zero-active rule, plan §0): no credential
                                // file, no fallback — so the card says so
                                // instead of pretending an old login works.
                                .when(!accounts.is_empty(), |card| {
                                    card.child(
                                        div()
                                            .flex()
                                            .flex_col()
                                            .gap(px(4.0))
                                            .when(!any_active, |list| {
                                                list.child(
                                                    div()
                                                        .text_size(px(11.5))
                                                        .text_color(theme.text_secondary)
                                                        .child(
                                                            "No account selected — choose one.",
                                                        ),
                                                )
                                            })
                                            .children(accounts.iter().map(|account| {
                                                let is_active = account.active.unwrap_or(false);
                                                let busy = row_busy
                                                    .as_ref()
                                                    .map(|id| id == &account.id)
                                                    .unwrap_or(false);
                                                let highlighted = recent
                                                    .as_ref()
                                                    .map(|id| id == &account.id)
                                                    .unwrap_or(false);
                                                account_row(
                                                    &theme,
                                                    &provider_id,
                                                    account,
                                                    is_active,
                                                    highlighted,
                                                    busy,
                                                    on_use.clone(),
                                                    on_remove.clone(),
                                                    on_rename.clone(),
                                                )
                                            })),
                                    )
                                })
                        }),
                ),
            )
            // Pasted-key providers render as their own rows below the OAuth
            // ones. They are filtered out of the list above so they do not get
            // a Login button they cannot use.
            .child(div().flex().flex_col().gap(px(10.0)).children(self.api_keys))
            // GitHub sits below the LLM providers as a sibling section, not
            // inside the provider list: it is a git credential, not a chat
            // provider, and its auth method (pasted token) has no equivalent
            // among the catalog entries above.
            .child(match self.github {
                Some(github) => div().mt(px(8.0)).child(github).into_any_element(),
                None => div().into_any_element(),
            })
            .into_any_element()
    }
}

/// The rename-account dialog, rendered in the shared settings modal shell.
/// The input arrives pre-seeded with the account's display name; Save hands
/// it to the caller (which issues the PATCH). A failed PATCH keeps the
/// dialog open with the server's error text visible, so the name the user
/// typed is still there to fix and resubmit.
#[derive(IntoElement)]
pub struct RenameAccountModal {
    pub input: Option<Entity<ComposerInput>>,
    /// True while the rename PATCH is in flight: Save reads "Renaming...".
    pub busy: bool,
    /// Server-authored failure text for the last rename attempt, verbatim.
    pub error: Option<String>,
    pub on_close: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
    pub on_save: Rc<dyn Fn(&mut Window, &mut App) + 'static>,
}

impl RenderOnce for RenameAccountModal {
    fn render(self, _window: &mut Window, cx: &mut App) -> impl IntoElement {
        let theme = Theme::current(cx);
        let on_close = self.on_close.clone();
        let busy = self.busy;

        let content = div()
            .flex()
            .flex_col()
            .gap(px(4.0))
            .child(
                div()
                    .text_size(px(11.5))
                    .font_weight(gpui::FontWeight::MEDIUM)
                    .text_color(theme.text_secondary)
                    .child("Account name"),
            )
            .when_some(self.input.clone(), |el, input| {
                el.child(
                    div()
                        .p(px(6.0))
                        .rounded(px(6.0))
                        .border_1()
                        .border_color(theme.border)
                        .bg(theme.canvas)
                        .child(input),
                )
            })
            .when_some(self.error.clone(), |el, error| {
                el.child(
                    div()
                        .text_size(px(11.5))
                        .text_color(theme.danger)
                        .child(error),
                )
            });

        let footer = div()
            .flex()
            .items_center()
            .justify_end()
            .gap(px(8.0))
            .child(
                div()
                    .px(px(10.0))
                    .py(px(4.0))
                    .rounded(px(5.0))
                    .border_1()
                    .border_color(theme.border_strong)
                    .bg(theme.raised)
                    .cursor_pointer()
                    .hover(|s| s.bg(theme.overlay))
                    .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                        cx.stop_propagation();
                        (on_close)(window, cx);
                    })
                    .child(
                        div()
                            .text_size(px(11.5))
                            .text_color(theme.text_secondary)
                            .child("Cancel"),
                    ),
            )
            .child({
                let on_save = self.on_save.clone();
                div()
                    .id("btn-rename-save")
                    .px(px(10.0))
                    .py(px(4.0))
                    .rounded(px(5.0))
                    .when(!busy, |el| {
                        el.bg(theme.accent)
                            .cursor_pointer()
                            .hover(|s| s.opacity(0.9))
                    })
                    .on_mouse_down(MouseButton::Left, move |_event, window, cx| {
                        if busy {
                            return;
                        }
                        cx.stop_propagation();
                        (on_save)(window, cx);
                    })
                    .child(
                        div()
                            .text_size(px(11.5))
                            .font_weight(gpui::FontWeight::MEDIUM)
                            .text_color(theme.on_inverse)
                            .child(if busy { "Renaming..." } else { "Save" }),
                    )
            });

        super::Modal {
            title: "Rename Account".to_string(),
            width: 360.0,
            content: content.into_any_element(),
            footer: footer.into_any_element(),
            on_close: self.on_close.clone(),
        }
    }
}
