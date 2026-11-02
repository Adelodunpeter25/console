//! Log filtering for expected GPUI window-teardown noise.
//!
//! GPUI's Windows backend destroys a window before flushing every queued
//! dispatcher task. Tasks that touch the already-destroyed HWND then log
//! `Invalid window handle (0x80040102)`, `Invalid window handle.
//! (0x80070578)`, or `window not found` at ERROR. These are expected races in
//! the pinned GPUI revision, not application failures, and they otherwise
//! bury genuine errors in the log.
//!
//! The filter suppresses only those exact messages from GPUI's own targets.
//! Running with `RUST_LOG=debug` (or `trace`) disables the suppression so the
//! full teardown sequence is still available when debugging.

use log::{Level, LevelFilter, Log, Metadata, Record};

const TEARDOWN_TARGETS: [&str; 2] = ["gpui_windows::", "gpui::"];
const TEARDOWN_MESSAGES: [&str; 2] = ["Invalid window handle", "window not found"];

struct TeardownNoiseFilter {
    inner: env_logger::Logger,
    /// `RUST_LOG=debug`/`trace` opts back into the raw teardown messages.
    passthrough: bool,
}

impl TeardownNoiseFilter {
    fn is_expected_teardown(record: &Record<'_>) -> bool {
        if record.level() != Level::Error {
            return false;
        }
        if !TEARDOWN_TARGETS
            .iter()
            .any(|target| record.target().starts_with(target))
        {
            return false;
        }
        let message = record.args().to_string();
        TEARDOWN_MESSAGES
            .iter()
            .any(|expected| message.contains(expected))
    }
}

impl Log for TeardownNoiseFilter {
    fn enabled(&self, metadata: &Metadata<'_>) -> bool {
        self.inner.enabled(metadata)
    }

    fn log(&self, record: &Record<'_>) {
        if self.passthrough || !Self::is_expected_teardown(record) {
            self.inner.log(record);
        }
    }

    fn flush(&self) {
        self.inner.flush();
    }
}

/// Installs `logger` behind the teardown-noise filter.
///
/// Takes a built `env_logger::Logger` instead of calling `init()` so the
/// wrapper can sit between the `log` facade and the file/stderr sink.
pub fn install(logger: env_logger::Logger) {
    let passthrough = logger.filter() >= LevelFilter::Debug;
    log::set_max_level(logger.filter());
    let filter = TeardownNoiseFilter {
        inner: logger,
        passthrough,
    };
    if let Err(error) = log::set_boxed_logger(Box::new(filter)) {
        eprintln!("failed to install logger: {error}");
    }
}
