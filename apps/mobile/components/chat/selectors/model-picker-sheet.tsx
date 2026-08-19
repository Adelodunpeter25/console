import React, { useRef, useState, useEffect } from "react";
import { View, Text, Pressable, ScrollView, TextInput, ActivityIndicator } from "react-native";
import { BottomSheetModal } from "@gorhom/bottom-sheet";
import { Sparkles, Check, Search, Bot } from "lucide-react-native";
import type { Model, ProviderId } from "@console/types";
import { SharedBottomSheet } from "../../common/shared-bottom-sheet";
import { useProviderStore } from "../../../stores";
import { theme } from "../../../styles/theme";

interface ModelPickerSheetProps {
  value: string | null;
  provider?: string | null;
  onChange: (modelId: string, provider?: ProviderId) => void;
}

function formatModelName(modelId: string): string {
  return modelId
    .split(/[-_]/g)
    .map((part) => (part.length > 0 ? part[0]!.toUpperCase() + part.slice(1) : part))
    .join(" ");
}

export function ModelPickerSheet({ value, provider, onChange }: ModelPickerSheetProps) {
  const bottomSheetRef = useRef<BottomSheetModal>(null);
  const providers = useProviderStore((state) => state.providers);
  const modelsByProvider = useProviderStore((state) => state.modelsByProvider);
  const loadProviders = useProviderStore((state) => state.loadProviders);
  const loadModels = useProviderStore((state) => state.loadModels);
  const loadingProviders = useProviderStore((state) => state.loadingProviders);

  const [activeProvider, setActiveProvider] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  useEffect(() => {
    void loadProviders();
  }, [loadProviders]);

  const handleOpen = () => {
    const defaultProv = provider || providers[0]?.name || null;
    setActiveProvider(defaultProv);
    setSearch("");
    if (defaultProv && !modelsByProvider[defaultProv]) {
      void loadModels(defaultProv).catch(() => {});
    }
    bottomSheetRef.current?.present();
  };

  const handleSelectProvider = (provName: string) => {
    setActiveProvider(provName);
    if (!modelsByProvider[provName]) {
      void loadModels(provName).catch(() => {});
    }
  };

  const currentModels: Model[] = activeProvider ? modelsByProvider[activeProvider] ?? [] : [];
  const query = search.trim().toLowerCase();
  const filteredModels = currentModels.filter(
    (m) => !query || m.id.toLowerCase().includes(query) || (m.name && m.name.toLowerCase().includes(query)),
  );

  return (
    <>
      <Pressable
        className="flex-row items-center gap-1.5 px-3 py-1.5 rounded-lg bg-card-alt/70 border border-border/50"
        style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
        onPress={handleOpen}
      >
        <Sparkles size={13} color={theme.colors.text.secondary} />
        <Text className="text-xs font-medium text-foreground max-w-[120px]" numberOfLines={1}>
          {value ? formatModelName(value) : "Default Model"}
        </Text>
      </Pressable>

      <SharedBottomSheet ref={bottomSheetRef} title="Select Model" snapPoints={["65%"]}>
        <View className="flex-1">
          {/* Provider tabs */}
          <ScrollView horizontal showsHorizontalScrollIndicator={false} className="mb-3">
            <View className="flex-row gap-1.5">
              {providers.map((p) => {
                const isSelected = p.name === activeProvider;
                return (
                  <Pressable
                    key={p.name}
                    className={`px-3 py-1.5 rounded-lg border ${
                      isSelected
                        ? "bg-card-alt border-border text-foreground"
                        : "bg-transparent border-transparent text-foreground-secondary"
                    }`}
                    onPress={() => handleSelectProvider(p.name)}
                  >
                    <Text
                      className={`text-xs font-medium ${
                        isSelected ? "text-foreground font-semibold" : "text-foreground-secondary"
                      }`}
                    >
                      {p.displayName || p.name}
                    </Text>
                  </Pressable>
                );
              })}
            </View>
          </ScrollView>

          {/* Search bar */}
          <View className="flex-row items-center bg-card-alt rounded-xl px-3 py-2 border border-border/50 mb-3">
            <Search size={14} color={theme.colors.text.muted} className="mr-2" />
            <TextInput
              value={search}
              onChangeText={setSearch}
              placeholder="Search models…"
              placeholderTextColor={theme.colors.text.muted}
              className="flex-1 text-xs text-foreground py-0.5"
            />
          </View>

          {/* Models list */}
          <ScrollView showsVerticalScrollIndicator={false} className="flex-1">
            {loadingProviders ? (
              <View className="items-center justify-center py-10">
                <ActivityIndicator size="small" color={theme.colors.text.muted} />
              </View>
            ) : filteredModels.length === 0 ? (
              <View className="items-center justify-center py-10">
                <Bot size={20} color={theme.colors.text.muted} />
                <Text className="text-xs text-foreground-secondary mt-1">No models available</Text>
              </View>
            ) : (
              filteredModels.map((model) => {
                const isSelected = model.id === value;
                return (
                  <Pressable
                    key={model.id}
                    className={`flex-row items-center justify-between px-3.5 py-2.5 rounded-xl mb-1.5 ${
                      isSelected ? "bg-card-alt border border-border/60" : ""
                    }`}
                    style={({ pressed }) => ({ opacity: pressed ? 0.7 : 1 })}
                    onPress={() => {
                      onChange(model.id, (activeProvider ?? undefined) as ProviderId | undefined);
                      bottomSheetRef.current?.dismiss();
                    }}
                  >
                    <View className="flex-1 mr-2">
                      <Text className="text-xs font-semibold text-foreground">{formatModelName(model.id)}</Text>
                      <Text className="text-[10px] text-foreground-secondary" numberOfLines={1}>
                        {model.id}
                      </Text>
                    </View>
                    {isSelected ? <Check size={14} color={theme.colors.status.ready} /> : null}
                  </Pressable>
                );
              })
            )}
          </ScrollView>
        </View>
      </SharedBottomSheet>
    </>
  );
}
