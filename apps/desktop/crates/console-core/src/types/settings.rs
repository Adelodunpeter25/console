// Canonical wire types from the shared protobuf schema
// (proto/console/v1/settings.proto). prost field names match the old
// hand-written structs, so construction sites are unchanged; the only
// difference is that model_roles is optional, matching proto3 message
// semantics. serde impls come from pbjson (protojson naming).
pub use console_proto::{ConsoleSettings, ModelRoleMapping};
