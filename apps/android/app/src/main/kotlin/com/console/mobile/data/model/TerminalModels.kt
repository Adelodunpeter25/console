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
