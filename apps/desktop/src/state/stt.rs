//! Speech-to-text credential state for the Providers settings page, and the
//! composer dictation flow that records and transcribes.
//!
//! Separate from state/provider_credentials.rs, which is the single pasted
//! key for LLM providers. Speech-to-text needs a pair (Cloudflare account ID
//! and API token) for a server-side Whisper model — not chat credentials — so
//! it gets its own section and its own state, modeled on GitHub's section
//! (state/github.rs).
//!
//! Neither half is retained in app state. The pair goes from the two text
//! inputs into the request body and is dropped when the request completes;
//! what is cached is only the token-free status.

use super::ConsoleDesktopApp;
use gpui::Context;
use std::time::Duration;

impl ConsoleDesktopApp {
    /// Ask the server for the stored speech-to-text credential state.
    ///
    /// Reported, not silent, for GitHub's reason: "Not connected" must mean
    /// "no credential", and a failed fetch rendering the same text made an
    /// unreachable backend indistinguishable from a disconnected account.
    pub fn refresh_stt_status(&mut self, cx: &mut Context<Self>) {
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.stt.status().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match result {
                            Ok(status) => {
                                this.stt_status = Some(status);
                                this.stt_error = None;
                            }
                            Err(error) => {
                                this.stt_error = Some(error.to_string());
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Store the Cloudflare account pair on the server.
    ///
    /// Both halves are required on every save: the token is never returned,
    /// so a partial update cannot preserve it. The pair is written into the
    /// request body and never retained — the caller drops it as soon as
    /// this returns. On success the cached status replaces it.
    pub fn save_stt_config(&mut self, account_id: String, api_token: String, cx: &mut Context<Self>) {
        let account_id = account_id.trim().to_string();
        let api_token = api_token.trim().to_string();
        if account_id.is_empty() || api_token.is_empty() {
            self.stt_error = Some("A Cloudflare account ID and API token are both required.".to_string());
            cx.notify();
            return;
        }
        if self.stt_busy {
            return;
        }

        self.stt_busy = true;
        self.stt_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.stt.set_config(&account_id, &api_token).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        match result {
                            Ok(response) => {
                                if let Some(status) = response.status {
                                    this.stt_status = Some(status);
                                }
                                this.stt_error = None;
                            }
                            // The server's wording is already written to be
                            // shown as-is, so pass it through rather than
                            // rewriting it.
                            Err(error) => {
                                this.stt_error = Some(error.to_string());
                            }
                        }
                        this.stt_busy = false;
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Remove the stored credential. Safe to call when already disconnected.
    pub fn disconnect_stt(&mut self, cx: &mut Context<Self>) {
        if self.stt_busy {
            return;
        }
        self.stt_busy = true;
        self.stt_error = None;
        cx.notify();

        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.stt.clear_config().await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.stt_busy = false;
                        // Drop the cached status either way: if the remove
                        // call failed but the credential is gone, a stale
                        // "Connected" row would be worse than showing
                        // disconnected.
                        this.stt_status = None;
                        if let Err(error) = result {
                            this.stt_error = Some(error.to_string());
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Show or hide the credentials form.
    pub fn set_stt_form_open(&mut self, open: bool, cx: &mut Context<Self>) {
        self.stt_form_open = open;
        // Closing the form must not leave the previous failure on screen.
        if !open {
            self.stt_error = None;
        }
        cx.notify();
    }

    /// The composer's mic button is enabled only when the server can actually
    /// transcribe — an unconfigured button that fails on tap is worse than a
    /// visibly disabled one.
    pub fn stt_enabled(&self) -> bool {
        self.stt_status.as_ref().and_then(|s| s.configured) == Some(true)
    }

    /// Mic button click: start a take when idle, stop + transcribe when
    /// recording. While a transcription is in flight the button is inert.
    pub fn toggle_stt_recording(&mut self, pane_id: String, cx: &mut Context<Self>) {
        if self.stt_transcribing {
            return;
        }
        if self.stt_recorder.is_some() {
            self.stop_speech_recording(cx);
            return;
        }
        if !self.stt_enabled() {
            return;
        }
        self.start_speech_recording(pane_id, cx);
    }

    fn start_speech_recording(&mut self, pane_id: String, cx: &mut Context<Self>) {
        self.stt_mic_error = None;
        let recorder = match crate::recording::Recorder::start() {
            Ok(recorder) => recorder,
            Err(error) => {
                self.stt_mic_error = Some(format!(
                    "Couldn't access the microphone. {error} In System Settings, allow microphone access for Console."
                ));
                cx.notify();
                return;
            }
        };
        self.stt_recorder = Some(recorder);
        self.stt_recording_pane = Some(pane_id);
        self.stt_recording_started_at = Some(std::time::Instant::now());
        self.stt_recording_secs = 0;
        cx.notify();
        self.tick_speech_recording(cx);
    }

    /// Keep the elapsed label repainting while a take is running. The loop
    /// exits on its own when the recorder is gone — no thread handle to join.
    fn tick_speech_recording(&mut self, cx: &mut Context<Self>) {
        cx.spawn(async move |entity, cx| {
            loop {
                cx.background_executor().timer(Duration::from_millis(250)).await;
                let keep_going = cx
                    .update(|cx| {
                        entity.upgrade().map(|this| {
                            this.update(cx, |this, cx| {
                                if this.stt_recorder.is_none() {
                                    return false;
                                }
                                let elapsed = this
                                    .stt_recording_started_at
                                    .map(|at| at.elapsed().as_secs() as u32)
                                    .unwrap_or(0);
                                // The server rejects >4 MB audio; 120s at
                                // 16 kHz mono 16-bit is ~3.8 MB.
                                if u64::from(elapsed) >= crate::recording::MAX_RECORD_SECONDS {
                                    this.stt_recording_secs = elapsed;
                                    this.stop_speech_recording(cx);
                                    return false;
                                }
                                if elapsed != this.stt_recording_secs {
                                    this.stt_recording_secs = elapsed;
                                    cx.notify();
                                }
                                true
                            })
                        })
                    })
                    .unwrap_or(false);
                if !keep_going {
                    break;
                }
            }
        })
        .detach();
    }

    /// Stop the take and transcribe it. The audio never touches disk: it is
    /// encoded to WAV in memory, sent once, and dropped.
    fn stop_speech_recording(&mut self, cx: &mut Context<Self>) {
        let Some(recorder) = self.stt_recorder.take() else {
            return;
        };
        let pane_id = self.stt_recording_pane.take();
        self.stt_recording_started_at = None;
        self.stt_recording_secs = 0;

        let wav = match recorder.stop_into_wav() {
            Ok(wav) if !wav.is_empty() => wav,
            Ok(_) => {
                self.stt_mic_error =
                    Some("Nothing was recorded — check the microphone and try again.".to_string());
                cx.notify();
                return;
            }
            Err(error) => {
                self.stt_mic_error = Some(error.to_string());
                cx.notify();
                return;
            }
        };

        self.stt_transcribing = true;
        cx.notify();
        let client = self.client.clone();
        cx.spawn(async move |entity, cx| {
            let result = client.stt.transcribe(&wav).await;
            let _ = cx.update(|cx| {
                if let Some(app) = entity.upgrade() {
                    app.update(cx, |this, cx| {
                        this.stt_transcribing = false;
                        match (result, pane_id.as_deref()) {
                            (Ok(transcript), Some(pane)) if !transcript.text.trim().is_empty() => {
                                this.insert_transcript(pane, &transcript.text, cx);
                                this.stt_mic_error = None;
                            }
                            (Ok(_), _) => {
                                this.stt_mic_error =
                                    Some("I couldn't hear anything — try again.".to_string());
                            }
                            (Err(error), _) => {
                                this.stt_mic_error =
                                    Some(format!("Transcription failed: {error}"));
                            }
                        }
                        cx.notify();
                    });
                }
            });
        })
        .detach();
    }

    /// Append the transcript to the pane's draft. Existing text is kept, so
    /// dictating mid-draft composes rather than clobbers.
    fn insert_transcript(&mut self, pane_id: &str, text: &str, cx: &mut Context<Self>) {
        let text = text.trim();
        let composer = self.composer_for_pane(pane_id);
        let existing = composer.read(cx).content().to_string();
        let body = if existing.trim().is_empty() {
            text.to_string()
        } else {
            format!("{} {}", existing, text)
        };
        composer.update(cx, |input, cx| input.set_content(body, cx));
    }
}
