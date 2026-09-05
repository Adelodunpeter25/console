import { useEffect } from "react";
import { Platform } from "react-native";
import * as Notifications from "expo-notifications";
import { app$, openChatSession } from "@/stores/useAppStore";
import { useNotificationStream } from "@/hooks/useNotificationStream";

Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
});

async function ensurePermissions(): Promise<boolean> {
  const current = await Notifications.getPermissionsAsync();
  if (current.granted || current.ios?.status === Notifications.IosAuthorizationStatus.PROVISIONAL) return true;
  const req = await Notifications.requestPermissionsAsync();
  return req.granted || req.ios?.status === Notifications.IosAuthorizationStatus.PROVISIONAL;
}

/**
 * Local-only notifications driven by the existing SSE stream.
 * No FCM/APNs/Expo-push infra: fires on-device while the app is
 * foregrounded or backgrounded (not swiped away).
 * Suppresses when already viewing that session; tap opens the session.
 */
export function useLocalNotifications(): void {
  useEffect(() => {
    let mounted = true;
    (async () => {
      if (Platform.OS === "android") {
        await Notifications.setNotificationChannelAsync("agent-activity", {
          name: "Agent activity",
          importance: Notifications.AndroidImportance.HIGH,
          vibrationPattern: [0, 250, 250, 250],
          lightColor: "#FFFFFF",
        });
      }
      if (mounted) await ensurePermissions().catch(() => false);
    })();
    const sub = Notifications.addNotificationResponseReceivedListener((response) => {
      const sessionId = (response.notification.request.content.data as { sessionId?: string } | undefined)?.sessionId;
      if (sessionId) openChatSession(sessionId);
    });
    return () => {
      mounted = false;
      sub.remove();
    };
  }, []);

  useNotificationStream((event) => {
    const viewingSame =
      app$.activeTab.peek() === "chat" && app$.selectedSessionId.peek() === event.sessionId;
    if (viewingSame) return;
    void Notifications.scheduleNotificationAsync({
      content: {
        title: event.title,
        body: event.body,
        data: { sessionId: event.sessionId, kind: event.kind },
      },
      trigger: null,
    }).catch(() => {});
  });
}
