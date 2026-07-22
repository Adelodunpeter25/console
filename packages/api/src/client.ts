import axios, { AxiosInstance, InternalAxiosRequestConfig } from "axios";

export interface ConsoleApiClientOptions {
  baseUrl?: string;
  getToken?: () => string | null | Promise<string | null>;
}

let clientInstance: AxiosInstance | null = null;
let currentBaseUrl: string = "http://localhost:3000";
let tokenGetter: (() => string | null | Promise<string | null>) | null = null;

/**
 * Configure the global API client settings (Base URL & Auth token supplier)
 */
export function configureConsoleApi(options: ConsoleApiClientOptions): AxiosInstance {
  if (options.baseUrl) {
    currentBaseUrl = options.baseUrl;
  }
  if (options.getToken !== undefined) {
    tokenGetter = options.getToken;
  }

  clientInstance = axios.create({
    baseURL: currentBaseUrl,
    headers: {
      "Content-Type": "application/json",
    },
  });

  clientInstance.interceptors.request.use(async (config: InternalAxiosRequestConfig) => {
    if (tokenGetter) {
      const token = await tokenGetter();
      if (token && config.headers) {
        config.headers.Authorization = `Bearer ${token}`;
      }
    }
    return config;
  });

  return clientInstance;
}

/**
 * Get the underlying Axios instance (initializes default client if not already configured)
 */
export function getConsoleApiClient(): AxiosInstance {
  if (!clientInstance) {
    return configureConsoleApi({});
  }
  return clientInstance;
}
