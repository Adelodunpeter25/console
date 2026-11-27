package com.console.mobile.core.notification

import android.content.Context
import com.console.mobile.data.repo.ChatRepository
import com.console.mobile.data.repo.NotificationRepository
import com.console.mobile.data.repo.SessionRepository
import com.console.mobile.data.store.AppStateHolder
import com.console.mobile.data.store.MobileTab
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/**
 * Listens to backend SSE notifications and coordinates app state updates
 * (suggestions, unread state, background work) and system notification banners.
 */
class NotificationCoordinator(
    private val context: Context,
    private val notificationRepository: NotificationRepository,
    private val chatRepository: ChatRepository,
    private val sessionRepository: SessionRepository,
    private val appStateHolder: AppStateHolder,
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    fun start() {
        // notifications() is a one-shot callbackFlow: the server closing the SSE
        // connection (idle timeout, restart, brief network drop) completes the flow
        // and collect() simply returns. Reconnect with backoff so the stream stays live.
        scope.launch {
            var backoffMs = 1_000L
            while (isActive) {
                try {
                    // A push sent while the stream was down is gone, but the server
                    // kept the set, so ask for the open chat's again.
                    appStateHolder.state.value.selectedSessionId?.let { chatRepository.loadSuggestions(it) }
                    notificationRepository.notifications().collect { event ->
                        if (event.kind == "suggestions") {
                            // Silent: chips for the chat that just finished, no banner.
                            chatRepository.setSuggestions(event.session_id, event.items)
                            return@collect
                        }
                        if (event.kind == "resumed") {
                            // Silent: an idle session woke itself for finished background
                            // work. The old run stream ended, so re-attach to the new run
                            // (after a beat, so the old run's sessionEnd lands first).
                            launch {
                                delay(400)
                                chatRepository.attachServerRun(event.session_id)
                            }
                            return@collect
                        }
                        if (event.kind == "read_state") {
                            // Silent: another device read or unread a chat. Refresh the
                            // list so the dot follows without waiting for the next poll.
                            sessionRepository.loadSessions()
                            return@collect
                        }
                        if (event.kind == "session_deleted") {
                            // Silent: the chat was deleted elsewhere or by the
                            // settled-chat sweep. Drop it locally, no banner.
                            sessionRepository.onSessionDeletedRemotely(event.session_id)
                            return@collect
                        }
                        if (event.kind == "background_work") {
                            // Silent: detached shell started, stopped or finished in the background.
                            chatRepository.loadBackgroundWork(event.session_id)
                            return@collect
                        }
                        if (event.kind == "done") {
                            // Refresh sessions so the unread dot appears on Home immediately.
                            sessionRepository.loadSessions()
                        }

                        // Notifications post when the app is in the background or when
                        // the user is looking at a different session/tab.
                        val isForeground = AppForegroundTracker.isForeground.value
                        val viewingSameChat = isForeground &&
                            appStateHolder.state.value.activeTab == MobileTab.Chat &&
                            appStateHolder.state.value.selectedSessionId == event.session_id

                        if (!viewingSameChat) {
                            LocalNotificationPresenter.showNotification(
                                context,
                                event.title,
                                event.body,
                                event.session_id,
                                event.subtitle,
                            )
                        }
                    }
                    backoffMs = 1_000L // a clean end-of-stream shouldn't escalate
                } catch (e: Exception) {
                    if (e is kotlinx.coroutines.CancellationException) throw e
                }
                delay(backoffMs)
                backoffMs = (backoffMs * 2).coerceAtMost(30_000L)
            }
        }
    }
}
