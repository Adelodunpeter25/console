// Assist types moved to the shared protobuf schema: SlashCommandInfo and
// FileSearchResponse are now console_proto types (see
// proto/console/v1/assist.proto). Re-exported here so `crate::types::*` and
// existing `use ...::types::SlashCommandInfo` imports keep resolving.
pub use console_proto::{AssistFileSearchResponse as FileSearchResponse, SlashCommandInfo};