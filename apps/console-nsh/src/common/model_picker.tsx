// The provider/model picker, ported from console-ui's
// `common/model_picker.rs`: the dropdown card with a provider icon sidebar
// (favourites star + one tab per provider), a search filter, and the model
// rows with favourite stars. The catalog arrives from the backend through
// console-core; until that service moves over (Rust pass) the app passes
// its catalog snapshot as props.

import { Input } from "nsh/base";
import { List, Text, View, token } from "nsh";
import type { JSX } from "nsh";
import { Icon } from "./icons";
import { ICON_ASSETS } from "./icons.generated";
import { formatContextWindow, formatModelName } from "../utils/format";
export { formatContextWindow, formatModelName } from "../utils/format";

/// A catalog model (`Model` of console-core; the wire fields the picker
/// reads).
export type CatalogModel = {
  id: string;
  contextWindow: number;
};

/// A provider of the catalog (`ProviderCatalogEntry`).
export type CatalogProvider = {
  name: string;
  displayName: string;
  models?: CatalogModel[];
};

/// The provider/model pair a session runs on (`SelectedModel`).
export type SelectedModel = {
  provider: string;
  modelId: string;
};

export type PickerTab = { kind: "favorites" } | { kind: "provider"; name: string };

// Provider logos exist for the known set (`provider_svg_path`); anything
// else gets the sparkle.
export function providerIconName(provider: string): Parameters<typeof Icon>[0]["name"] {
  const norm = provider.toLowerCase();
  if (norm === "antigravity" || norm === "google") return "provider/Antigravity";
  if (norm === "openai" || norm === "codex" || norm === "chatgpt") return "provider/Openai";
  if (norm === "opencode") return "provider/Opencode";
  if (norm === "claude" || norm === "anthropic") return "ProviderClaude";
  if (norm === "deepseek") return "ProviderDeepseek";
  if (norm === "grok") return "ProviderGrok";
  if (norm === "cursor") return "ProviderCursor";
  if (norm === "amp") return "ProviderAmp";
  if (norm === "pi") return "ProviderPi";

  const candidate = provider[0].toUpperCase() + provider.slice(1).toLowerCase();
  return candidate in ICON_ASSETS
    ? (candidate as Parameters<typeof Icon>[0]["name"])
    : "Sparkle";
}

/// The dropdown card (`ModelDropdownMenu`): provider tabs on the left,
/// search and the filtered model list on the right. Favourites tab empty
/// state shows "No Favourites"; a provider's empty list "No models found".
export function ModelDropdownMenu(props: {
  providers: CatalogProvider[];
  // Accessors, not values: the popover mounts its content once, so reactive
  // values have to be read through functions that close over the owning
  // signals (see the lazy-prop note in composer.tsx).
  selected: () => SelectedModel | null | undefined;
  activeTab: () => PickerTab;
  favorites: () => Set<string>;
  searchQuery: () => string;
  onSearch: (query: string) => void;
  onSelect: (provider: string, modelId: string) => void;
  onTab: (tab: PickerTab) => void;
  onFavorite: (provider: string, modelId: string) => void;
}) {
  const query = props.searchQuery().trim().toLowerCase();
  // Live-fetched models per provider override the static catalog list; the
  // picker is never blank. The NSH side receives the resolved list, as the
  // desktop's `models_by_provider` snapshot arrives merged in `providers`.
  // A thunk: the list refilters when the query or tab changes, and reads
  // stay tracked only inside JSX (see the lazy-prop note in composer.tsx).
  const visibleModels = (): { provider: CatalogProvider; model: CatalogModel }[] => {
    const query = props.searchQuery().trim().toLowerCase();
    const activeTab = props.activeTab();
    const favorites = props.favorites();
    const active = props.providers.find(
      (p) => activeTab.kind === "provider" && p.name === activeTab.name,
    );
    let rows =
      activeTab.kind === "favorites"
        ? props.providers.flatMap((p) =>
            (p.models ?? [])
              .filter((m) => favorites.has(`${p.name}:${m.id}`))
              .map((m) => ({ provider: p, model: m })),
          )
        : (active?.models ?? []).map((m) => ({ provider: active!, model: m }));
    // The filter checks `model.id.contains(query)`, like the desktop picker.
    if (query) {
      rows = rows.filter(({ model }) => model.id.toLowerCase().includes(query));
    }
    return rows;
  };

  const tabClasses = (active: boolean) =>
    "flex h-[38px] w-[38px] flex-row items-center justify-center rounded-md " +
    (active ? "bg-muted" : "hover:bg-muted");

  return (
    <View class="flex h-[350px] w-[400px] flex-row overflow-hidden rounded-[13px] border border-border bg-background">
      <View class="flex w-[50px] flex-none flex-col items-center gap-1 border-r border-border bg-background p-1">
        <View
          class={tabClasses(props.activeTab().kind === "favorites")}
          onPress={() => props.onTab({ kind: "favorites" })}
          accessibilityLabel="Favourites"
          role="button"
        >
          <Icon
            name="Star"
            size={16}
            color={
              props.activeTab().kind === "favorites"
                ? token("foreground")
                : token("muted-foreground")
            }
          />
        </View>
        <View class="my-0.5 h-px w-[34px] bg-border" />
        {props.providers.map((provider) => {
          const tab = props.activeTab();
          const active =
            tab.kind === "provider" && tab.name === provider.name;
          return (
            <View
              class={tabClasses(active)}
              onPress={() => props.onTab({ kind: "provider", name: provider.name })}
              accessibilityLabel={provider.displayName}
              role="button"
            >
              <Icon
                name={providerIconName(provider.name)}
                size={17}
                mono={false}
                color={active ? token("foreground") : token("muted-foreground")}
              />
            </View>
          );
        })}
      </View>
      <View class="flex flex-1 flex-col overflow-hidden">
        <View class="mx-2 mt-2 flex h-[30px] flex-none flex-row items-center gap-1.5 rounded-md border border-border bg-muted px-2">
          <Icon name="Search" size={13} color={token("muted-foreground")} />
          <Input
            class="flex-1 text-xs text-foreground"
            placeholder="Search models…"
            value={props.searchQuery()}
            onChange={(e: { value: string }) => props.onSearch(e.value)}
            testID="model-search"
          />
        </View>
        <View class="flex flex-1 flex-col overflow-hidden p-2">
          {visibleModels().length === 0 ? (
            <View class="flex flex-1 flex-col items-center justify-center gap-1">
              {props.activeTab().kind !== "favorites" ? (
                <Icon name="Star" size={18} color={token("muted-foreground")} />
              ) : null}
              <Text class="text-[11.5px] text-muted-foreground">
                {props.activeTab().kind === "favorites"
                  ? "No Favourites"
                  : "No models found"}
              </Text>
            </View>
          ) : (
            <List
              class="h-full w-full"
              each={visibleModels()}
              keyExtractor={(item) => `${item.provider.name}:${item.model.id}`}
            >
              {(row) => {
                const { provider, model } = row();
                const selected = props.selected();
                const isSelected =
                  selected?.provider === provider.name &&
                  selected?.modelId === model.id;
                const isFav = props.favorites().has(`${provider.name}:${model.id}`);
                return (
                  <View
                    class={
                      "flex h-[54px] min-h-[54px] flex-none flex-row items-center justify-between rounded-lg px-3 " +
                      (isSelected ? "bg-muted" : "hover:bg-muted")
                    }
                    onPress={() => props.onSelect(provider.name, model.id)}
                    accessibilityLabel={formatModelName(model.id)}
                    role="button"
                  >
                    <View class="flex min-w-0 flex-1 flex-col gap-0.5">
                      <Text class="truncate text-[13px] font-semibold text-foreground">
                        {formatModelName(model.id)}
                      </Text>
                      <View class="flex flex-row items-center gap-1">
                        <Icon
                          name={providerIconName(provider.name)}
                          size={10.5}
                          mono={false}
                        />
                        <Text class="truncate text-[11px] text-muted-foreground">
                          {provider.displayName} ·{" "}
                          {formatContextWindow(model.contextWindow)} context
                        </Text>
                      </View>
                    </View>
                    <View
                      class="rounded p-1 hover:bg-muted"
                      onPress={() => props.onFavorite(provider.name, model.id)}
                      accessibilityLabel={
                        isFav ? "Unfavourite" : "Favourite"
                      }
                      role="button"
                    >
                      <Icon
                        name={isFav ? "StarFilled" : "Star"}
                        size={13}
                        color={isFav ? token("primary") : token("muted-foreground")}
                      />
                    </View>
                  </View>
                );
              }}
            </List>
          )}
        </View>
      </View>
    </View>
  );
}
