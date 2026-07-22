import React, { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { consoleQueryClient } from "./query-client.js";
import { configureConsoleApi, ConsoleApiClientOptions } from "./client.js";

export interface ConsoleApiProviderProps extends ConsoleApiClientOptions {
  children: ReactNode;
  queryClient?: QueryClient;
}

export const ConsoleApiProvider: React.FC<ConsoleApiProviderProps> = ({
  children,
  baseUrl,
  getToken,
  queryClient = consoleQueryClient,
}) => {
  if (baseUrl !== undefined || getToken !== undefined) {
    configureConsoleApi({ baseUrl, getToken });
  }

  return (
    <QueryClientProvider client={queryClient}>
      {children}
    </QueryClientProvider>
  );
};
