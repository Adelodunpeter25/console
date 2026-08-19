import React from "react";
import { Text, View, TouchableOpacity, ScrollView, ActivityIndicator } from "react-native";
import type { OAuthProviderId } from "@console/types";
import { Check } from "lucide-react-native";
import { GlassSurface } from "../../components/layout/glass-surface";
import { useAuth } from "../../hooks";
import { useProviderCatalog } from "../../hooks";

export function AccountSettings() {
  const auth = useAuth();
  const catalog = useProviderCatalog();

  const handleLogin = async (provider: OAuthProviderId) => {
    try {
      await auth.login(provider);
    } catch (err) {
      console.error("Failed to open login URL:", err);
    }
  };

  const oauthProviders = catalog.providers.filter((p) => p.authMethod === "oauth");

  return (
    <ScrollView
      style={{ flex: 1 }}
      contentContainerStyle={{ paddingHorizontal: 16, paddingTop: 8, paddingBottom: 40 }}
      showsVerticalScrollIndicator={false}
    >
      <View className="mb-4 px-1">
        <Text className="text-sm text-foreground-secondary">
          Sign in to AI providers to use their models in chat.
        </Text>
      </View>

      {catalog.loadingProviders && oauthProviders.length === 0 ? (
        <View className="items-center py-8">
          <ActivityIndicator size="small" color="#ffffff" />
        </View>
      ) : (
        <GlassSurface className="p-5">
          {catalog.providers.map((p, i) => {
            const provider = p.name;
            const isOauth = p.authMethod === "oauth";
            const isDeviceCode = p.authMethod === "device-code";
            const isFree = p.authMethod === "none";
            const loggedIn = auth.isLoggedIn(provider as OAuthProviderId);

            return (
              <View
                key={provider}
                className={`flex-row items-center justify-between py-3.5 ${
                  i < catalog.providers.length - 1 ? "border-b border-border/50" : ""
                }`}
              >
                <View className="flex-1 pr-3">
                  <Text className="text-sm font-semibold text-foreground">{p.displayName || p.name}</Text>
                  <Text className="text-xs text-foreground-secondary mt-0.5" numberOfLines={2}>
                    {p.description ||
                      (isFree
                        ? "Free provider (no login required)"
                        : loggedIn
                          ? (auth.status?.[provider as OAuthProviderId]?.email ?? "Connected")
                          : "Not logged in")}
                  </Text>
                </View>

                {isFree ? (
                  <View className="px-3 py-1 rounded-full bg-card-alt border border-border">
                    <Text className="text-xs font-semibold text-foreground-secondary">Free Access</Text>
                  </View>
                ) : loggedIn ? (
                  <View className="flex-row items-center gap-1 px-3 py-1 rounded-full bg-emerald-500/10 border border-emerald-500/20">
                    <Check size={12} color="#34d399" />
                    <Text className="text-xs font-bold text-emerald-400">Connected</Text>
                  </View>
                ) : (
                  <TouchableOpacity
                    className="px-4 py-2 rounded-full bg-foreground items-center justify-center"
                    onPress={() => {
                      if (isOauth) handleLogin(provider as OAuthProviderId);
                    }}
                    disabled={auth.isFetchingLoginUrl}
                  >
                    {auth.isFetchingLoginUrl ? (
                      <ActivityIndicator size="small" color="#000000" />
                    ) : (
                      <Text className="text-xs font-bold text-black">
                        {isDeviceCode ? "Pair Device" : "Log In"}
                      </Text>
                    )}
                  </TouchableOpacity>
                )}
              </View>
            );
          })}
        </GlassSurface>
      )}
    </ScrollView>
  );
}

export default AccountSettings;
