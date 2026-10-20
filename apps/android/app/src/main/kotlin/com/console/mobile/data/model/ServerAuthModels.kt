package com.console.mobile.data.model

/**
 * The server-pairing payloads are shared protobuf types (`console.v1`, see
 * proto/console/v1/auth.proto), so they are decoded through the Wire
 * adapters rather than hand-written DTOs.
 *
 * The request bodies are shared too: `PairDeviceRequest` carries the one-time
 * pairing grant, and `PairingGrantRequest` asks the server for a grant
 * another device can redeem.
 */
typealias PairingRequest = console.v1.PairDeviceRequest
typealias PairingData = console.v1.PairDeviceResponse
typealias PairedDevice = console.v1.PairedDevice
typealias PairingGrantRequest = console.v1.PairingGrantRequest
typealias PairingGrantData = console.v1.PairingGrantResponse
typealias DeviceInfo = console.v1.ServerDevice
typealias DeviceListData = console.v1.ServerDevicesResponse
