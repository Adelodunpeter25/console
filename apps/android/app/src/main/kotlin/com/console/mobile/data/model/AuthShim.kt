package com.console.mobile.data.model

// ProviderAuthStatus moved to the shared protobuf schema (console.v1 from
// proto/console/v1/auth.proto) as console.v1.ProviderAuthStatus. The status
// shim below is now console.v1.AuthStatusResponse: every provider is a
// nullable Wire message (the server always sends all four, so in practice
// they are present), and AuthRepository unwraps them with ?:
// console.v1.ProviderAuthStatus(..) ?: console.v1.ProviderAuthStatus.Builder()
//     .setLoggedIn(false)
//     .build()
