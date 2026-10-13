//! Integration DTOs mirroring the server's `/api/integrations` contract.
//!
//! The wire types are shared protobuf (console_proto, see
//! proto/console/v1/integrations.proto) and re-exported below. The list is
//! open vocabulary — the server appends rows as new binaries need detecting,
//! so clients must render unknown ids rather than switching on them.

pub use console_proto::{IntegrationStatus, IntegrationsResponse};
