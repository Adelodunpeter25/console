// The app's icons: the same SVG icon set the GPUI app ships
// (`apps/desktop/assets/icons/`), drawn with <Image> over a data: URI and
// recolored with `tintColor`. Add a name by copying that file's paths.

import { Image, type ColorValue } from "nsh";

const PATHS = {
  "chat-round-line": `<path d="M12 22C17.5228 22 22 17.5228 22 12C22 6.47715 17.5228 2 12 2C6.47715 2 2 6.47715 2 12C2 13.5997 2.37562 15.1116 3.04346 16.4525C3.22094 16.8088 3.28001 17.2161 3.17712 17.6006L2.58151 19.8267C2.32295 20.793 3.20701 21.677 4.17335 21.4185L6.39939 20.8229C6.78393 20.72 7.19121 20.7791 7.54753 20.9565C8.88837 21.6244 10.4003 22 12 22Z"/><path d="M8 10.5H16"/><path d="M8 14H13.5"/>`,
  alert: `<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 20h16a2 2 0 0 0 1.73-2Z"/><path d="M12 9v4"/><path d="M12 17h.01"/>`,
} as const;

export type IconName = keyof typeof PATHS;

function uri(name: IconName): string {
  const body = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="black" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">${PATHS[name]}</svg>`;
  return `data:image/svg+xml,${encodeURIComponent(body)}`;
}

/// A monochrome icon of the app's set, drawn in one color.
export function Icon(props: {
  name: IconName;
  size?: number;
  color?: ColorValue;
}) {
  const size = props.size ?? 16;
  return (
    <Image
      src={uri(props.name)}
      style={{ width: size, height: size, tintColor: props.color }}
    />
  );
}
