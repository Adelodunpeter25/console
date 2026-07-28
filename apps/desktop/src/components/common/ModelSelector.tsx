import React from "react";
import { useProviderStore } from "../../store";
import { Dropdown, DropdownItem, DropdownGroupHeading } from "./Dropdown";

interface ModelSelectorProps {
  value: string | null;
  onChange: (modelId: string) => void;
}

/**
 * Dropdown for selecting an LLM model. Pulls providers from the provider
 * store and loads models on first open. Built on the shared Dropdown
 * primitives so it shares the same visual language as other selectors.
 */
export function ModelSelector({ value, onChange }: ModelSelectorProps) {
  const { providers, modelsByProvider, loadProviders, loadModels, loadingProviders, loadingModels } =
    useProviderStore();

  React.useEffect(() => {
    loadProviders();
  }, [loadProviders]);

  const allModels = React.useMemo(() => {
    return providers.flatMap((p) => {
      const models = modelsByProvider[p.name] ?? [];
      return models.map((m) => ({ ...m, providerName: p.displayName }));
    });
  }, [providers, modelsByProvider]);

  const handleOpen = () => {
    providers.forEach((p) => {
      if (!modelsByProvider[p.name]) {
        loadModels(p.name).catch(() => {});
      }
    });
  };

  return (
    <Dropdown
      label={value ?? "Default"}
      heading="Models"
      onOpen={handleOpen}
      width={264}
    >
      {loadingProviders && (
        <div className="px-3 py-4 text-center text-xs text-foreground-muted">Loading...</div>
      )}
      {!loadingProviders && allModels.length === 0 && (
        <div className="px-3 py-4 text-center text-xs text-foreground-muted">
          No models available
        </div>
      )}
      {providers.map((provider) => {
        const models = modelsByProvider[provider.name] ?? [];
        const isLoading = loadingModels[provider.name];
        return (
          <div key={provider.name}>
            <DropdownGroupHeading>{provider.displayName}</DropdownGroupHeading>
            {isLoading ? (
              <div className="px-3 py-2 text-xs text-foreground-muted">Loading models...</div>
            ) : (
              models.map((model) => (
                <DropdownItem
                  key={model.id}
                  selected={value === model.id}
                  onClick={() => onChange(model.id)}
                >
                  {model.id}
                </DropdownItem>
              ))
            )}
          </div>
        );
      })}
    </Dropdown>
  );
}
