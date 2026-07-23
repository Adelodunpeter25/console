import { useQuery } from "@tanstack/react-query";
import { providerService } from "../services/provider.service";

export const providerKeys = {
  all: ["providers"] as const,
  models: (providerId: string) => ["providers", providerId, "models"] as const,
};

export function useProviders() {
  return useQuery({
    queryKey: providerKeys.all,
    queryFn: () => providerService.getProviders(),
  });
}

export function useProviderModels(providerId: string) {
  return useQuery({
    queryKey: providerKeys.models(providerId),
    queryFn: () => providerService.getProviderModels(providerId),
    enabled: Boolean(providerId),
  });
}
