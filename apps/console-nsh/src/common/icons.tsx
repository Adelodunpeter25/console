// The app's icons: the GPUI app's whole icon set
// (`apps/desktop/assets/icons/`), mapped like console-ui's `icons.rs`
// (`primitives::IconName` → file) in `icons.generated.ts`. Drawn with
// <Image> and recolored with `tintColor`; provider and file-type logos
// don't recolor (they are brand marks), so pass `mono: false` for those.

import { Image, type ColorValue } from "nsh";
import { ICON_ASSETS, type IconName } from "./icons.generated";

export type { IconName };

/// A monochrome icon of the app's set, drawn in one color.
export function Icon(props: {
  name: IconName;
  size?: number;
  color?: ColorValue;
  /// false for provider and file-type logos, which keep their brand colors.
  mono?: boolean;
}) {
  const asset = ICON_ASSETS[props.name];
  const size = props.size ?? 16;
  return (
    <Image
      src={asset.url}
      style={{
        width: size,
        height: size,
        tintColor: props.mono === false ? undefined : props.color,
      }}
    />
  );
}
