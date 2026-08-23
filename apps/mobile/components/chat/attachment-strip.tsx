import React, { useState } from "react";
import { View, Pressable, Image } from "react-native";
import { X } from "lucide-react-native";
import type { ImageAttachment } from "@console/types";
import { ImagePreviewModal } from "../common/image-preview-modal";

interface AttachmentStripProps {
  attachments: ImageAttachment[];
  onRemove: (index: number) => void;
}

export function AttachmentStrip({ attachments, onRemove }: AttachmentStripProps) {
  const [previewImageUri, setPreviewImageUri] = useState<string | null>(null);

  if (attachments.length === 0) return null;

  return (
    <>
      <View className="flex-row flex-wrap gap-2 px-2 pb-2">
        {attachments.map((att, idx) => {
          const imageUri = `data:${att.mimeType};base64,${att.data}`;
          return (
            <View
              key={idx}
              className="relative rounded-xl overflow-hidden border border-white/20 bg-card"
            >
              <Pressable
                onPress={() => setPreviewImageUri(imageUri)}
                style={({ pressed }) => ({ opacity: pressed ? 0.8 : 1 })}
              >
                <Image source={{ uri: imageUri }} className="w-14 h-14" resizeMode="cover" />
              </Pressable>
              <Pressable
                onPress={() => onRemove(idx)}
                className="absolute top-1 right-1 w-5 h-5 rounded-full bg-black/80 items-center justify-center border border-white/20"
                hitSlop={6}
              >
                <X size={11} color="#ffffff" />
              </Pressable>
            </View>
          );
        })}
      </View>

      <ImagePreviewModal
        visible={Boolean(previewImageUri)}
        imageUri={previewImageUri}
        onClose={() => setPreviewImageUri(null)}
      />
    </>
  );
}
