import React, { memo } from "react";
import { SvgXml } from "react-native-svg";
import { getProviderIconXml } from "../../utils/icons/provider-icons";

interface ProviderIconProps {
  provider: string;
  size?: number;
}

/**
 * Renders a provider logo (antigravity, codebuff, gemini, openai, opencode,
 * with `codex` aliased to openai). Returns null for unknown providers.
 */
export const ProviderIcon = memo(function ProviderIcon({
  provider,
  size = 14,
}: ProviderIconProps) {
  const xml = getProviderIconXml(provider);
  if (!xml) return null;
  return <SvgXml xml={xml} width={size} height={size} />;
});
