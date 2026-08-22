import React, { memo } from "react";
import { SvgXml } from "react-native-svg";
import { FILE_ICONS, type FileIconName } from "../../utils/icons/file-type-registry";
import { getFileIconXml } from "../../utils/icons/file-type-mapping";

interface FileIconProps {
  /** Filename or full path — resolved via extension / exact-name rules. */
  filename?: string;
  /** Explicit registry key, bypassing filename resolution (e.g. code-fence languages). */
  iconKey?: FileIconName;
  size?: number;
}

/**
 * Renders the Material file-type icon for a filename, path, or explicit
 * registry key. Falls back to a generic file icon for unknown types.
 */
export const FileIcon = memo(function FileIcon({
  filename = "",
  iconKey,
  size = 14,
}: FileIconProps) {
  const xml = iconKey ? FILE_ICONS[iconKey] : getFileIconXml(filename);
  return <SvgXml xml={xml} width={size} height={size} />;
});
