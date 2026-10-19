package com.console.mobile.feature.ports

import com.console.mobile.core.forward.LocalForward
import console.v1.ForwardedPort

/**
 * One line of the ports list: a port the server forwards, plus the loopback
 * tunnel this device may have opened for it.
 *
 * The two are independent — the server can report a port the phone never
 * tunnelled (the phone is on a different network, or the listener failed to
 * bind) — which is exactly what the row's status shows.
 */
data class PortRow(
    val port: Int,
    val projectId: String?,
    /** The server's own proxy URL: LAN-reachable, useless off that network. */
    val lanUrl: String?,
    /** A `127.0.0.1` URL, set once this device's listener is up. */
    val localUrl: String?,
) {
    val tunneled: Boolean get() = localUrl != null
}

/**
 * Joins the server's forwarded ports with this device's live tunnels, in the
 * order they are shown (ascending port, so a stack reads like a stack).
 *
 * A port with no tunnel is still listed: it exists server-side, and tapping it
 * is what asks [com.console.mobile.data.repo.PortsRepository] to open one.
 */
fun portRows(ports: List<ForwardedPort>, locals: List<LocalForward>): List<PortRow> =
    ports.sortedBy { it.port }.map { port ->
        PortRow(
            port = port.port,
            projectId = port.project_id?.takeIf { it.isNotBlank() },
            lanUrl = port.url.takeIf { it.isNotBlank() },
            localUrl = locals.firstOrNull { it.remotePort == port.port }?.localUrl,
        )
    }
