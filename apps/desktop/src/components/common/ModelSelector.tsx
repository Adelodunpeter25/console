import React from "react";
import {
  Bot,
  Check,
  Code2,
  LoaderCircle,
  Search,
  Sparkles,
  Star,
  WandSparkles,
  type LucideIcon,
} from "lucide-react";
import type { Model, ProviderCatalogEntry, ProviderId } from "@console/types";
import { useModelFavorites, modelFavoriteKey } from "../../hooks/useModelFavorites";
import { useProviderStore } from "../../store/useProviderStore";
import { Dropdown } from "./Dropdown";

interface ModelSelectorProps {
  value: string | null;
  provider?: string | null;
  onChange: (modelId: string, provider?: ProviderId) => void;
}

type PickerTab = ProviderId | "favorites";

const PROVIDER_ICONS: Record<ProviderId, LucideIcon> = {
  gemini: Sparkles,
  antigravity: WandSparkles,
  opencode: Code2,
  codebuff: Bot,
};

const PROVIDER_IMAGES: Partial<Record<ProviderId, string>> = {
  gemini: "/providers/gemini.svg",
  antigravity: "/providers/antigravity.svg",
  opencode: "/providers/opencode.svg",
  codebuff: "/providers/codebuff.svg",
};

const PROVIDER_MASK_IMAGES = new Set<ProviderId>(["antigravity", "opencode", "codebuff"]);

function formatModelName(modelId: string): string {
  return modelId
    .split(/[-_]/g)
    .map((part) => (part.length > 0 ? part[0]!.toUpperCase() + part.slice(1) : part))
    .join(" ");
}

function providerModels(
  provider: ProviderCatalogEntry,
  modelsByProvider: Record<string, Model[]>,
): Model[] {
  return modelsByProvider[provider.name] ?? provider.models;
}

/**
 * Provider-filtered model picker with server-persisted favorites.
 * Provider and model data remain owned by the provider store/backend.
 */
export function ModelSelector({ value, provider, onChange }: ModelSelectorProps) {
  const providers = useProviderStore((state) => state.providers);
  const modelsByProvider = useProviderStore((state) => state.modelsByProvider);
  const loadProviders = useProviderStore((state) => state.loadProviders);
  const loadModels = useProviderStore((state) => state.loadModels);
  const loadingProviders = useProviderStore((state) => state.loadingProviders);
  const loadingModels = useProviderStore((state) => state.loadingModels);
  const [search, setSearch] = React.useState("");
  const [activeTab, setActiveTab] = React.useState<PickerTab>("favorites");
  const [open, setOpen] = React.useState(false);
  const { favorites, favoriteSet, toggleFavorite: toggleModelFavorite } = useModelFavorites();

  const query = search.trim().toLowerCase();

  React.useEffect(() => {
    void loadProviders();
  }, [loadProviders]);

  React.useEffect(() => {
    if (provider && providers.some((item) => item.name === provider)) {
      setActiveTab(provider as ProviderId);
    }
  }, [provider, providers]);

  React.useEffect(() => {
    if (activeTab !== "favorites") {
      void loadModels(activeTab).catch(() => {});
    }
  }, [activeTab, loadModels]);

  const handleOpen = () => {
    providers.forEach((item) => {
      if (!modelsByProvider[item.name]) void loadModels(item.name).catch(() => {});
    });
  };

  const getVisibleModels = (): Array<{ model: Model; provider: ProviderCatalogEntry }> => {
    const selectedProviders =
      activeTab === "favorites"
        ? providers
        : providers.filter((item) => item.name === activeTab);

    return selectedProviders.flatMap((item) =>
      providerModels(item, modelsByProvider)
        .filter((model) => {
          if (activeTab === "favorites" && !favoriteSet.has(modelFavoriteKey(item.name, model.id))) {
            return false;
          }
          return !query || model.id.toLowerCase().includes(query);
        })
        .map((model) => ({ model, provider: item })),
    );
  };

  const visibleModels = getVisibleModels();
  const activeProvider = providers.find((item) => item.name === activeTab);

  const selectTab = (tab: PickerTab) => {
    setActiveTab(tab);
    setSearch("");
  };

  return (
    <Dropdown
      label={value ? formatModelName(value) : "Default"}
      heading="Models"
      onOpen={handleOpen}
      width={480}
      scrollable={false}
      open={open}
      onOpenChange={setOpen}
    >
      <div className="flex h-[min(350px,calc(100vh-8rem))] min-h-0">
        <aside className="flex w-12 shrink-0 flex-col items-center gap-1 overflow-y-auto border-r border-white/[0.08] pr-1">
          <button
            type="button"
            onClick={() => selectTab("favorites")}
            title="Favorites"
            aria-label="Favorites"
            className={`flex h-9 w-9 items-center justify-center rounded-md transition-colors ${
              activeTab === "favorites"
                ? "bg-white/[0.1] text-foreground"
                : "text-foreground-secondary hover:bg-white/[0.06] hover:text-foreground"
            }`}
          >
            <Star size={15} className={favorites.length > 0 ? "text-yellow-400" : ""} />
          </button>

          <div className="my-1 h-px w-7 bg-white/[0.08]" />

          {providers.map((item) => {
            const Icon = PROVIDER_ICONS[item.name];
            const image = PROVIDER_IMAGES[item.name];
            return (
              <button
                key={item.name}
                type="button"
                onClick={() => selectTab(item.name)}
                aria-label={item.displayName}
                className={`flex h-9 w-9 items-center justify-center rounded-md transition-colors ${
                  activeTab === item.name
                    ? "bg-white/[0.1] text-foreground"
                    : "text-foreground-secondary hover:bg-white/[0.06] hover:text-foreground"
                }`}
              >
                {image && PROVIDER_MASK_IMAGES.has(item.name) ? (
                  <span
                    aria-hidden="true"
                    className="h-4 w-4 bg-current"
                    style={{
                      WebkitMaskImage: `url(${image})`,
                      maskImage: `url(${image})`,
                      WebkitMaskPosition: "center",
                      maskPosition: "center",
                      WebkitMaskRepeat: "no-repeat",
                      maskRepeat: "no-repeat",
                      WebkitMaskSize: "contain",
                      maskSize: "contain",
                    }}
                  />
                ) : image ? (
                  <img src={image} alt="" className="h-4 w-4" />
                ) : (
                  <Icon size={15} />
                )}
              </button>
            );
          })}
        </aside>

        <section className="flex min-w-0 flex-1 flex-col pl-3">
          <div className="relative mb-2 shrink-0">
            <Search
              size={14}
              className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-foreground-muted"
            />
            <input
              autoFocus
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder={activeTab === "favorites" ? "Search favorites..." : "Search models..."}
              className="h-8 w-full rounded-md border border-white/[0.12] bg-[#1d1d1d] pl-8 pr-2 text-xs text-foreground outline-none placeholder:text-foreground-muted focus:border-white/[0.28] focus:ring-1 focus:ring-white/[0.06]"
            />
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto pr-1">
            {loadingProviders ? (
              <div className="flex h-full items-center justify-center gap-2 text-xs text-foreground-muted">
                <LoaderCircle size={14} className="animate-spin" /> Loading providers...
              </div>
            ) : visibleModels.length === 0 ? (
              <div className="flex h-full flex-col items-center justify-center gap-1 px-4 text-center text-xs text-foreground-muted">
                <Star size={18} />
                <span>
                  {activeTab === "favorites"
                    ? "Star a model to add it to Favorites"
                    : activeProvider
                      ? `No models found for ${activeProvider.displayName}`
                      : "No providers available"}
                </span>
              </div>
            ) : (
              visibleModels.map(({ model, provider: modelProvider }) => {
                const favorite = favoriteSet.has(modelFavoriteKey(modelProvider.name, model.id));
                const selected = value === model.id;
                const loading = loadingModels[modelProvider.name];
                return (
                  <div
                    key={`${modelProvider.name}:${model.id}`}
                    className={`group mb-1 flex items-center rounded-md transition-colors ${
                      selected ? "bg-white/[0.12]" : "hover:bg-white/[0.06]"
                    }`}
                  >
                    <button
                      type="button"
                      onClick={() => {
                        onChange(model.id, modelProvider.name);
                        setOpen(false);
                      }}
                      className="min-w-0 flex-1 px-2.5 py-2 text-left outline-none focus-visible:ring-1 focus-visible:ring-white/[0.35]"
                    >
                      <span className="flex items-center gap-2 text-xs font-medium text-foreground">
                        <span className="min-w-0 flex-1 truncate" title={model.id}>
                          {formatModelName(model.id)}
                        </span>
                        {selected && <Check size={14} className="shrink-0 text-success" />}
                      </span>
                      <span className="mt-0.5 block truncate text-[10px] text-foreground-muted">
                        {activeTab === "favorites" ? modelProvider.displayName : model.id}
                        {loading && <LoaderCircle size={10} className="ml-1 inline animate-spin" />}
                      </span>
                    </button>
                    <button
                      type="button"
                      aria-label={favorite ? `Remove ${model.id} from favorites` : `Add ${model.id} to favorites`}
                      onClick={() => toggleModelFavorite(model)}
                      className={`mr-1.5 rounded p-1.5 outline-none transition-colors focus-visible:ring-1 focus-visible:ring-white/[0.35] ${
                        favorite
                          ? "text-yellow-400"
                          : "text-foreground-muted opacity-60 hover:text-yellow-400 group-hover:opacity-100"
                      }`}
                    >
                      <Star size={15} fill={favorite ? "currentColor" : "none"} />
                    </button>
                  </div>
                );
              })
            )}
          </div>
        </section>
      </div>
    </Dropdown>
  );
}
