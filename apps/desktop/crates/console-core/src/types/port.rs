//! Forwarded port models for local & remote development servers.
//!
//! Canonical wire types from the shared protobuf schema
//! (proto/console/v1/ports.proto). Ports are int32 on the wire (JSON
//! numbers, as before); UI layers bridging to u16 socket APIs cast at the
//! boundary. serde impls come from pbjson (protojson naming).
pub use console_proto::{ForwardPortRequest, ForwardedPort};
