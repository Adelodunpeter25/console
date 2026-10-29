// The pure helpers live in `console-shared` (one implementation, shared with
// the Android client); these modules keep the old paths working.
pub mod diff {
    pub use console_shared::diff::*;
}
pub mod file_kind {
    pub use console_shared::file_kind::*;
}
pub mod pairing_url {
    pub use console_shared::pairing::*;
}
pub mod http_transport;
mod page_title;
pub mod server_auth_store;
pub mod sse_reader;

pub use diff::{
    DiffLine, DiffLineKind, DiffResult, FileDiff, diff_lines, extract_edit_args,
    extract_write_args, extract_write_files, file_call_diffs,
};
pub use file_kind::{
    BLOCKED_FILE_EXTENSIONS, FileKind, MARKDOWN_EXTENSIONS, RASTER_IMAGE_EXTENSIONS,
    file_kind_for_path, is_lock_file,
};
pub use http_transport::{HttpTransport, decode_json_bytes, fetch_url_bytes, probe_backend};
pub use page_title::{extract_html_title, fetch_page_title};
pub use pairing_url::{PairingUrl, host_port, parse_pairing_url, split_pairing_line};
pub use server_auth_store::StoredServerAuth;
pub use sse_reader::SseStreamReader;
