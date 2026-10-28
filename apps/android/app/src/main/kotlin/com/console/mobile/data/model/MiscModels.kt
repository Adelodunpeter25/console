package com.console.mobile.data.model

import kotlinx.serialization.Serializable

// TodoItem moved to the shared protobuf schema (console.v1 from
// proto/console/v1): ids narrow to int32 and stay JSON numbers.

object TodoStatus {
    const val PENDING = "pending"
    const val IN_PROGRESS = "in_progress"
    const val COMPLETED = "completed"
}

// NotificationEvent moved to the shared protobuf schema
// (console.v1.NotificationEvent from proto/console/v1): the SSE stream has
// no envelope, so the frame decodes straight off the wire.

// Usage quota types moved to the shared protobuf schema (console.v1 from
// proto/console/v1): trimmed to what the UI renders, with unit/status as
// plain strings. The old client-side resolveUsedFraction is gone — the
// server derives status/used_percent/reset_label (ToProtoReport), so the
// clients only render those.
