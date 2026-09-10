//! Small, reusable, side-effect-free helpers (time formatting, grouping, …).

pub mod path;
pub mod session_groups;
pub mod time;

pub use path::{format_folder_display_name, join_path_lexical};
pub use session_groups::{
    ProjectSectionKey, SessionDateGroup, SidebarSortMode, group_by_date, group_indices_by_date,
    group_indices_by_project, session_date_group,
};
pub use time::{format_message_time, format_time_ago, format_working_elapsed};
