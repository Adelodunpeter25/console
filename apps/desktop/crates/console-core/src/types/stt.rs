//! Speech-to-text DTOs mirroring the server's `/api/stt` contracts.
//!
//! The wire types are shared protobuf (console_proto, see
//! proto/console/v1/stt.proto) and re-exported below. The status carries
//! only *whether* a credential exists, which account it is, and where it
//! came from — never the token, which lives on the server and is not
//! readable through any endpoint.

pub use console_proto::{SetSttConfigResponse, SttStatus, SttTranscript};
