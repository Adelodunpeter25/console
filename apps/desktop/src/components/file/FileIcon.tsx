import React from "react";
import { getBuiltInSpriteSheet } from "@pierre/trees";
import { resolveFileIconToken } from "../../utils/file-icons";

const FULL_SPRITE_SHEET_SVG = getBuiltInSpriteSheet("complete");

let spriteInjected = false;
function ensureSpriteSheetInjected() {
  if (spriteInjected || typeof document === "undefined") return;
  spriteInjected = true;
  const container = document.createElement("div");
  container.id = "file-icon-sprite-sheet";
  container.style.display = "none";
  container.innerHTML = FULL_SPRITE_SHEET_SVG;
  document.body.appendChild(container);
}

interface FileIconProps {
  fileName: string;
  className?: string;
}

/**
 * FileIcon — Renders @pierre/trees sprite SVG icon for any file name or path.
 */
export function FileIcon({ fileName, className = "w-3.5 h-3.5 shrink-0" }: FileIconProps) {
  React.useEffect(() => {
    ensureSpriteSheetInjected();
  }, []);

  const token = resolveFileIconToken(fileName);

  return (
    <svg className={className}>
      <use href={`#file-tree-builtin-${token}`} />
    </svg>
  );
}
