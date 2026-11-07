// The image preview overlay, ported from console-ui's
// `viewer/image_preview.rs`: a transcript image's press opens the picture
// over the workspace; a press anywhere closes it. The desktop's meta row
// (dimensions, byte size) and zoom need the image bytes; nsh's `Image`
// loads by URL, so the overlay shows the picture alone.
//
// The open image lives in a module signal so any transcript row can open
// the overlay without threading state through the pane tree.

import { signal, Image, Overlay, Text, View, token } from "nsh";
import { Icon } from "../common/icons";

export const previewImage = signal<string | null>(null);

/// Mount once in the shell; renders nothing until a transcript image opens
/// it (components run once in nsh, so the signal read sits inside JSX).
export function ImagePreviewOverlay() {
  return (
    <>
      {(() => {
        const uri = previewImage();
        if (uri == null) return null;
        return (
          <Overlay class="flex-1 items-center justify-center bg-black/80 p-8">
            <View
              class="flex-1 items-center justify-center gap-3"
              role="button"
              accessibilityLabel="Close image preview"
              onPress={() => previewImage.set(null)}
            >
              <Image src={uri} class="max-h-[80%] max-w-[80%] rounded-lg" />
              <View class="flex-row items-center gap-1.5 rounded border border-border bg-background px-2 py-1">
                <Icon name="X" size={12} color={token("muted-foreground")} />
                <Text class="text-[11px] text-muted-foreground">Close</Text>
              </View>
            </View>
          </Overlay>
        );
      })()}
    </>
  );
}
