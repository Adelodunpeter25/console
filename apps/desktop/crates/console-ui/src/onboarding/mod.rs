//! First-run setup, shown once per server.
//!
//! Takes over the entire main window — no title bar, no sidebar, no workspace —
//! the way the mobile connect screen does. An overlay floating on top of the
//! workspace looked like an interruption rather than a first-run experience, and
//! left the sidebar visible behind it.
//!
//! Live setup status mirrors `crate::state::onboarding::SetupStatus`, which this
//! library cannot name: `console-ui` must not depend on the app binary.

mod panel;

pub use panel::{OnboardingPanel, SetupStatus};
