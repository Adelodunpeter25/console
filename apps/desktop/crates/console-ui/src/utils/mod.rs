//! Small, reusable, side-effect-free helpers (time formatting, grouping, …).

pub mod session_groups;
pub mod time;

pub use session_groups::{
    SessionDateGroup, group_by_date, group_indices_by_date, session_date_group,
};
pub use time::{format_message_time, format_time_ago, format_working_elapsed};
