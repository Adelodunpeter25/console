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
          {oauthProviders.map((p, i) => {
            const provider = p.name as OAuthProviderId;
            const loggedIn = auth.isLoggedIn(provider);
            return (
              <View
                key={provider}
                className={`flex-row items-center justify-between py-3 ${
                  i < oauthProviders.length - 1 ? "border-b border-border" : ""
                }`}
              >
                <View className="flex-1 pr-3">
                  <Text className="text-sm font-semibold text-foreground">{p.name}</Text>
                  <Text className="text-xs text-foreground-secondary mt-0.5">
                    {loggedIn
                      ? (auth.status?.[provider]?.email ?? "Logged in")
                      : "Not logged in"}
                  </Text>
                </View>
                {loggedIn ? (
                  <View className="flex-row items-center gap-1 px-3 py-1 rounded-full bg-foreground/10 border border-border">
                    <Check size={12} color="#34d399" />
                    <Text className="text-xs font-bold text-foreground">Connected</Text>
                  </View>
                ) : (
                  <TouchableOpacity
                    className="px-4 py-2 rounded-full bg-foreground items-center justify-center"
                    onPress={() => handleLogin(provider)}
                    disabled={auth.isFetchingLoginUrl}
                  >
                    {auth.isFetchingLoginUrl ? (
                      <ActivityIndicator size="small" color="#000000" />
                    ) : (
                      <Text className="text-xs font-bold text-black">Log In</Text>
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
