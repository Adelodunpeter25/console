package com.console.mobile.data.model

import kotlinx.serialization.Serializable

typealias TerminalId = String

@Serializable
data class TerminalSpawnParams(
    val cwd: String,
    val shell: String? = null,
    val cols: Int? = null,
    val rows: Int? = null,
    val label: String? = null,
    val proto: String? = null,
)

// Terminal control frames moved to the shared protobuf schema (console.v1
// from proto/console/v1): oneof JSON shape, raw PTY bytes and the
// tag-byte framing untouched. Spawn params stay a URL query builder.

/**
 * Client→server control frames are `TerminalClientMessage` oneofs —
 * `{"resize":{"cols":..,"rows":..}}`, `{"kill":{}}`. Encoding the inner message
 * alone (`{"cols":..,"rows":..}`) is rejected by the server as an unknown
 * frame, so the PTY silently stays at its spawn size.
 */
private val clientFrameAdapter = com.squareup.moshi.Moshi.Builder()
    .add(com.squareup.wire.WireJsonAdapterFactory())
    .build()
    .adapter(console.v1.TerminalClientMessage::class.java)

fun encodeResizeFrame(cols: Int, rows: Int): String = clientFrameAdapter.toJson(
    console.v1.TerminalClientMessage(
        event = console.v1.TerminalClientMessage.Event.Resize(console.v1.TerminalResize(cols = cols, rows = rows)),
    ),
)

fun encodeKillFrame(): String = clientFrameAdapter.toJson(
    console.v1.TerminalClientMessage(
        event = console.v1.TerminalClientMessage.Event.Kill(console.v1.TerminalKill()),
    ),
)

const val OUTPUT_FRAME_TAG: Byte = 0x01
const val INPUT_FRAME_TAG: Byte = 0x01

fun buildTerminalWsUrl(baseUrl: String, params: TerminalSpawnParams): String {
    val wsBase = baseUrl.replaceFirst("http://", "ws://").replaceFirst("https://", "wss://")
    val cols = params.cols ?: 80
    val rows = params.rows ?: 24
    val sb = StringBuilder("$wsBase/api/terminals?cwd=${java.net.URLEncoder.encode(params.cwd, "UTF-8")}&cols=$cols&rows=$rows")
    if (params.shell != null) sb.append("&shell=${java.net.URLEncoder.encode(params.shell, "UTF-8")}")
    if (params.label != null) sb.append("&label=${java.net.URLEncoder.encode(params.label, "UTF-8")}")
    sb.append("&proto=binary")
    return sb.toString()
}
