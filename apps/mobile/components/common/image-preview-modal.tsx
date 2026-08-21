import React from "react";
import { Modal, View, Image, Pressable, Text, Dimensions, StatusBar } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { X } from "lucide-react-native";

interface ImagePreviewModalProps {
  visible: boolean;
  imageUri: string | null;
  title?: string;
  onClose: () => void;
}

export function ImagePreviewModal({
  visible,
  imageUri,
  title,
  onClose,
}: ImagePreviewModalProps) {
  const insets = useSafeAreaInsets();
  const { width, height } = Dimensions.get("window");

  if (!visible || !imageUri) return null;

  return (
    <Modal
      visible={visible}
      transparent
      animationType="fade"
      statusBarTranslucent
      onRequestClose={onClose}
    >
      <StatusBar barStyle="light-content" />
      <View className="flex-1 bg-black/95 justify-between">
        {/* Top Header Bar */}
        <View
          className="flex-row items-center justify-between px-5 z-10"
          style={{ paddingTop: Math.max(insets.top, 16) + 8, paddingBottom: 12 }}
        >
          <Text className="text-white/80 font-medium text-sm">
            {title || "Image Preview"}
          </Text>
          <Pressable
            onPress={onClose}
            className="w-9 h-9 rounded-full bg-white/10 items-center justify-center border border-white/15 active:bg-white/20"
            hitSlop={10}
          >
            <X size={18} color="#ffffff" />
          </Pressable>
        </View>

        {/* Center Image View */}
        <Pressable
          className="flex-1 items-center justify-center px-4"
          onPress={onClose}
        >
          <Image
            source={{ uri: imageUri }}
            style={{ width: width - 32, height: height * 0.72 }}
            resizeMode="contain"
          />
        </Pressable>

        {/* Bottom spacer / footer */}
        <View style={{ paddingBottom: Math.max(insets.bottom, 16) + 12 }} className="items-center">
          <Text className="text-white/40 text-xs">Tap anywhere to close</Text>
        </View>
      </View>
    </Modal>
  );
}
