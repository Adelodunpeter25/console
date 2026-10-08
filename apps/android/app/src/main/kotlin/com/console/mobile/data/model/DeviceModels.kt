package com.console.mobile.data.model

import console.v1.DeviceDescriptor

// Device types moved to the shared protobuf schema (console.v1 from
// proto/console/v1/device.proto): DeviceDescriptor, DeviceDiagnostics and
// DeviceActionRequest are Wire types now. Note diskFreeBytes is uint64, so it
// arrives as a protojson *string* (a real disk exceeds 4 GB) — Wire surfaces it
// as a Long, which is the same value.
//
// The UI-side helpers that used to hang off DeviceDescriptor are now extension
// properties below, so call sites keep reading `.isBooted`, `.isIos`, etc.

val DeviceDescriptor.isBooted: Boolean get() = state.equals("booted", ignoreCase = true)
val DeviceDescriptor.isBooting: Boolean get() = state.equals("booting", ignoreCase = true)
val DeviceDescriptor.isIos: Boolean get() = platform.equals("ios", ignoreCase = true)
val DeviceDescriptor.displayName: String get() = name.ifEmpty { id }
