import React, { useState } from "react";
import {
  Text,
  View,
  TouchableOpacity,
  ScrollView,
  ActivityIndicator,
  Alert,
} from "react-native";
import type { OAuthProviderId, ProviderId } from "@console/types";
import {
  Check,
  Circle,
  LogIn,
  RefreshCw,
  LoaderCircle,
} from "lucide-react-native";
import { GlassSurface } from "@/components/layout/glass-surface";
import { useAuth, useLocalOAuthLogin } from "@/hooks";
import { useProviderCatalog } from "@/hooks";
import { theme } from "@/styles/theme";

export function AccountSettings() {
  const auth = useAuth();
  const catalog = useProviderCatalog();
  const localOAuth = useLocalOAuthLogin();

  const [loggingInProvider, setLoggingInProvider] = useState<ProviderId | null>(null);

  const handleLogin = async (provider: ProviderId, authMethod?: string) => {
    setLoggingInProvider(provider);
    try {
      if (authMethod === "oauth" && localOAuth.isAvailable) {
        await localOAuth.loginWithLocalServer(provider as OAuthProviderId);
      } else if (authMethod === "oauth") {
        await auth.login(provider as OAuthProviderId);
      }
    } catch (err) {
      const message = err instanceof Error ? err.message : "Login failed.";
      Alert.alert("Login Failed", message);
    } finally {
      setLoggingInProvider(null);
    }
  };

  // Show all providers that require authentication (skip "none"/free).
  const providers = catalog.providers.filter((p) => p.authMethod !== "none");

  const isBusy = loggingInProvider !== null || localOAuth.isLoggingIn;

  return (
    <ScrollView
      style={{ flex: 1 }}
      contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 8, paddingBottom: 40 }}
      keyboardShouldPersistTaps="handled"
      keyboardDismissMode="on-drag"
      showsVerticalScrollIndicator={false}
    >
      <View className="mb-4 px-1">
        <Text className="text-sm text-foreground-secondary">
          Sign in to AI providers to use their models in chat.
        </Text>
      </View>

      {catalog.loadingProviders && providers.length === 0 ? (
        <View className="items-center py-8">
          <ActivityIndicator size="small" color="#ffffff" />
        </View>
      ) : (
        <GlassSurface className="p-5">
          {providers.map((p, i) => {
            const id = p.name as ProviderId;
            const isOauth = p.authMethod === "oauth";
            const isDeviceCode = p.authMethod === "device-code";
            const providerStatus = auth.status?.[id as OAuthProviderId];
            const loggedIn = Boolean(providerStatus?.loggedIn);
            const email = providerStatus?.email;
            const isThisProviderLoggingIn =
              isBusy && (loggingInProvider === id || (localOAuth.isLoggingIn && loggingInProvider === id));

            return (
              <View key={id}>
                <View
                  className={`flex-row items-center justify-between py-3.5 ${
                    i < providers.length - 1 ? "border-b border-border/50" : ""
                  }`}
                >
                  {/* Status icon + name + email */}
                  <View className="flex-1 pr-3 flex-row items-center">
                    {loggedIn ? (
                      <Check size={14} color="#34d399" />
                    ) : (
                      <Circle size={14} color={theme.colors.text.muted} />
                    )}
                    <View className="flex-1 ml-2.5">
                      <Text className="text-sm font-semibold text-foreground">
                        {p.displayName || p.name}
                      </Text>
                      <Text className="text-xs text-foreground-secondary mt-0.5" numberOfLines={1}>
                        {loggedIn ? (email ?? "Connected") : "Not connected"}
                      </Text>
                    </View>
                  </View>

                  {/* Login / Re-login button */}
                  <TouchableOpacity
                    className={`flex-row items-center gap-1.5 px-3.5 py-2 rounded-full items-center justify-center ${
                      loggedIn
                        ? "bg-transparent border border-border"
                        : "bg-foreground"
                    }`}
                    onPress={() => handleLogin(id, p.authMethod)}
                    disabled={isBusy}
                  >
                    {isThisProviderLoggingIn ? (
                      <LoaderCircle size={13} color={loggedIn ? "#ffffff" : "#000000"} />
                    ) : loggedIn ? (
                      <RefreshCw size={13} color="#ffffff" />
                    ) : (
                      <LogIn size={13} color="#000000" />
                    )}
                    <Text
                      className={`text-xs font-bold ${
                        loggedIn ? "text-foreground" : "text-black"
                      }`}
                    >
                      {isThisProviderLoggingIn
                        ? "Wait"
                        : loggedIn
                          ? "Re-login"
                          : isDeviceCode
                            ? "Pair"
                            : "Login"}
                    </Text>
                  </TouchableOpacity>
                </View>
              </View>
            );
          })}
        </GlassSurface>
      )}

      {(auth.error || localOAuth.error) && (
        <View className="mt-4 px-1">
          <Text className="text-xs text-red-400">
            {localOAuth.error || auth.error}
          </Text>
        </View>
      )}
    </ScrollView>
  );
}

export default AccountSettings;
