import { useQuery } from "@tanstack/react-query";
import { usageService } from "../services/usage.service";

export const usageKeys = {
  all: ["usage"] as const,
  provider: (providerId: string) => ["usage", providerId] as const,
};

export function useUsage(providerId: string) {
  return useQuery({
    queryKey: usageKeys.provider(providerId),
    queryFn: () => usageService.getProviderUsage(providerId),
    enabled: Boolean(providerId),
    staleTime: 30_000,
  });
}

export function useAllUsage() {
  return useQuery({
    queryKey: usageKeys.all,
    queryFn: () => usageService.getAllUsage(),
    staleTime: 30_000,
  });
}
