import React, { useCallback } from "react";
import { Modal, View, Pressable, Text, Dimensions, StatusBar } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import {
  GestureDetector,
  Gesture,
  GestureHandlerRootView,
} from "react-native-gesture-handler";
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  withTiming,
  runOnJS,
} from "react-native-reanimated";
import { X, ZoomIn } from "lucide-react-native";

interface ImagePreviewModalProps {
  visible: boolean;
  imageUri: string | null;
  title?: string;
  onClose: () => void;
}

const MIN_SCALE = 1;
const MAX_SCALE = 4;

export function ImagePreviewModal({
  visible,
  imageUri,
  title,
  onClose,
}: ImagePreviewModalProps) {
  const insets = useSafeAreaInsets();
  const { width, height } = Dimensions.get("window");

  const scale = useSharedValue(1);
  const savedScale = useSharedValue(1);
  const translateX = useSharedValue(0);
  const translateY = useSharedValue(0);
  const savedTranslateX = useSharedValue(0);
  const savedTranslateY = useSharedValue(0);

  const resetZoom = useCallback(() => {
    "worklet";
    scale.value = withSpring(1);
    savedScale.value = 1;
    translateX.value = withSpring(0);
    translateY.value = withSpring(0);
    savedTranslateX.value = 0;
    savedTranslateY.value = 0;
  }, [scale, savedScale, translateX, translateY, savedTranslateX, savedTranslateY]);

  const handleClose = () => {
    resetZoom();
    onClose();
  };

  // Pinch Gesture for multi-touch zoom
  const pinchGesture = Gesture.Pinch()
    .onUpdate((e) => {
      const nextScale = savedScale.value * e.scale;
      scale.value = Math.max(MIN_SCALE * 0.8, Math.min(nextScale, MAX_SCALE * 1.5));
    })
    .onEnd(() => {
      if (scale.value < MIN_SCALE) {
        resetZoom();
      } else if (scale.value > MAX_SCALE) {
        scale.value = withSpring(MAX_SCALE);
        savedScale.value = MAX_SCALE;
      } else {
        savedScale.value = scale.value;
      }
    });

  // Pan Gesture to move around when zoomed in
  const panGesture = Gesture.Pan()
    .averageTouches(true)
    .onUpdate((e) => {
      if (scale.value > 1) {
        // Clamp live so the image can't be dragged fully off-screen; onEnd
        // still springs back anything left out of bounds after scale changes.
        const maxTranslateX = (width * (scale.value - 1)) / 2;
        const maxTranslateY = (height * 0.72 * (scale.value - 1)) / 2;
        translateX.value = Math.max(
          -maxTranslateX,
          Math.min(savedTranslateX.value + e.translationX, maxTranslateX),
        );
        translateY.value = Math.max(
          -maxTranslateY,
          Math.min(savedTranslateY.value + e.translationY, maxTranslateY),
        );
      }
    })
    .onEnd(() => {
      if (scale.value > 1) {
        // Keep within reasonable bounds
        const maxTranslateX = (width * (scale.value - 1)) / 2;
        const maxTranslateY = (height * 0.72 * (scale.value - 1)) / 2;

        if (Math.abs(translateX.value) > maxTranslateX) {
          translateX.value = withSpring(Math.sign(translateX.value) * maxTranslateX);
        }
        if (Math.abs(translateY.value) > maxTranslateY) {
          translateY.value = withSpring(Math.sign(translateY.value) * maxTranslateY);
        }

        savedTranslateX.value = translateX.value;
        savedTranslateY.value = translateY.value;
      } else {
        translateX.value = withSpring(0);
        translateY.value = withSpring(0);
        savedTranslateX.value = 0;
        savedTranslateY.value = 0;
      }
    });

  // Double tap to zoom in 2.5x or reset to 1x
  const doubleTapGesture = Gesture.Tap()
    .numberOfTaps(2)
    .onEnd(() => {
      if (scale.value > 1.2) {
        resetZoom();
      } else {
        scale.value = withSpring(2.5);
        savedScale.value = 2.5;
      }
    });

  // Single tap on background to dismiss when not zoomed
  const singleTapGesture = Gesture.Tap()
    .numberOfTaps(1)
    .onEnd(() => {
      if (scale.value <= 1.05) {
        runOnJS(handleClose)();
      }
    });

  // Exclusive so single-tap dismiss can't win the race and swallow the
  // double-tap zoom; the tap pair then runs simultaneously with pinch/pan.
  // (Race alone let Tap(1) activate on the first release, blocking double-tap.)
  const composedGestures = Gesture.Simultaneous(
    Gesture.Exclusive(doubleTapGesture, singleTapGesture),
    pinchGesture,
    panGesture,
  );

  const animatedImageStyle = useAnimatedStyle(() => ({
    transform: [
      { translateX: translateX.value },
      { translateY: translateY.value },
      { scale: scale.value },
    ],
  }));

  if (!visible || !imageUri) return null;

  return (
    <Modal
      visible={visible}
      transparent
      animationType="fade"
      statusBarTranslucent
      onRequestClose={handleClose}
    >
      <StatusBar barStyle="light-content" />
      {/* RN Modals render outside the app's GestureHandlerRootView — gestures
          are completely dead inside a Modal unless it has its own root. */}
      <GestureHandlerRootView style={{ flex: 1 }}>
      <View className="flex-1 bg-black/95 justify-between">
        {/* Top Header Bar */}
        <View
          className="flex-row items-center justify-between px-5 z-20"
          style={{ paddingTop: Math.max(insets.top, 16) + 8, paddingBottom: 12 }}
        >
          <View className="flex-row items-center gap-2">
            <Text className="text-white/90 font-semibold text-sm">
              {title || "Image Preview"}
            </Text>
          </View>
          <Pressable
            onPress={handleClose}
            className="w-9 h-9 rounded-full bg-white/10 items-center justify-center border border-white/15 active:bg-white/25"
            hitSlop={10}
          >
            <X size={18} color="#ffffff" />
          </Pressable>
        </View>

        {/* Center Pinch / Zoom / Pan Image View */}
        <View className="flex-1 items-center justify-center overflow-hidden">
          <GestureDetector gesture={composedGestures}>
            <Animated.View className="items-center justify-center">
              <Animated.Image
                source={{ uri: imageUri }}
                style={[
                  {
                    width: width - 24,
                    height: height * 0.74,
                  },
                  animatedImageStyle,
                ]}
                resizeMode="contain"
              />
            </Animated.View>
          </GestureDetector>
        </View>

        {/* Bottom footer hint */}
        <View
          style={{ paddingBottom: Math.max(insets.bottom, 16) + 12 }}
          className="items-center z-20"
        >
          <Text className="text-white/40 text-xs">
            Pinch or double-tap to zoom • Drag to pan
          </Text>
        </View>
      </View>
      </GestureHandlerRootView>
    </Modal>
  );
}
