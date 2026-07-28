import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { authService } from "../services/auth.service.js";
import type { OAuthLoginUrlDto, OAuthCallbackDto } from "@console/types";

export const authKeys = {
  status: ["auth", "status"] as const,
};

export function useAuthStatus() {
  return useQuery({
    queryKey: authKeys.status,
    queryFn: () => authService.getAuthStatus(),
  });
}

export function useGetLoginUrl() {
  return useMutation({
    mutationFn: (payload: OAuthLoginUrlDto) => authService.getLoginUrl(payload),
  });
}

export function useHandleOAuthCallback() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: OAuthCallbackDto) => authService.handleCallback(payload),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: authKeys.status });
    },
  });
}
