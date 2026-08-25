import React, { useCallback, useRef } from "react";
import {
  Modal,
  View,
  Text,
  Pressable,
  Animated,
} from "react-native";
import { observable } from "@legendapp/state";
import { useValue } from "@legendapp/state/react";

export interface ConfirmDialogButton {
  text: string;
  style?: "default" | "cancel" | "destructive";
  onPress?: () => void | Promise<void>;
}

interface ConfirmDialogOptions {
  title: string;
  message?: string;
  buttons?: ConfirmDialogButton[];
}

interface ConfirmDialogOptions {
  title: string;
  message?: string;
  buttons?: ConfirmDialogButton[];
}

/** Dialog visibility state as Legend State observables. */
export const confirmDialog$ = observable({
  isOpen: false,
  options: null as ConfirmDialogOptions | null,
});

export function hideConfirmDialog(): void {
  confirmDialog$.isOpen.set(false);
  confirmDialog$.options.set(null);
}

/**
 * Drop-in replacement for React Native's Alert.alert()
 */
export function confirmAlert(
  title: string,
  message?: string,
  buttons?: ConfirmDialogButton[],
) {
  confirmDialog$.options.set({
    title,
    message,
    buttons: buttons && buttons.length > 0 ? buttons : [{ text: "OK", style: "default" }],
  });
  confirmDialog$.isOpen.set(true);
}

export function ConfirmDialog() {
  const isOpen = useValue(confirmDialog$.isOpen);
  const options = useValue(confirmDialog$.options);
  const opacity = useRef(new Animated.Value(0)).current;
  const scale = useRef(new Animated.Value(0.94)).current;

  const onShow = useCallback(() => {
    Animated.parallel([
      Animated.timing(opacity, {
        toValue: 1,
        duration: 160,
        useNativeDriver: true,
      }),
      Animated.spring(scale, {
        toValue: 1,
        damping: 24,
        stiffness: 320,
        useNativeDriver: true,
      }),
    ]).start();
  }, [opacity, scale]);

  const handleClose = useCallback(
    (onPress?: () => void | Promise<void>) => {
      Animated.parallel([
        Animated.timing(opacity, {
          toValue: 0,
          duration: 120,
          useNativeDriver: true,
        }),
        Animated.timing(scale, {
          toValue: 0.94,
          duration: 120,
          useNativeDriver: true,
        }),
      ]).start(() => {
        hideConfirmDialog();
        if (onPress) {
          setTimeout(() => {
            void onPress();
          }, 50);
        }
      });
    },
    [opacity, scale],
  );

  if (!isOpen || !options) {
    return null;
  }

  const buttons = options.buttons && options.buttons.length > 0
    ? options.buttons
    : [{ text: "OK", style: "default" as const }];

  const isRowLayout = buttons.length === 2;

  return (
    <Modal
      visible={isOpen}
      transparent
      animationType="none"
      statusBarTranslucent
      onShow={onShow}
      onRequestClose={() => handleClose()}
    >
      <Pressable
        className="flex-1 bg-black/65 justify-center items-center px-6"
        onPress={() => handleClose()}
      >
        <Animated.View
          style={{ opacity, transform: [{ scale }] }}
          className="w-full max-w-[330px] bg-[#16171a] rounded-[20px] border border-white/10 p-5 shadow-2xl"
          onStartShouldSetResponder={() => true}
        >
          <View className="mb-5">
            <Text className="text-[17px] font-semibold text-foreground text-center tracking-tight">
              {options.title}
            </Text>
            {options.message ? (
              <Text className="text-sm text-foreground-secondary text-center mt-2 leading-5">
                {options.message}
              </Text>
            ) : null}
          </View>

          <View className={isRowLayout ? "flex-row gap-2.5" : "gap-2"}>
            {buttons.map((btn, index) => {
              const isDestructive = btn.style === "destructive";
              const isCancel = btn.style === "cancel";
              const isPrimary = !isDestructive && !isCancel;

              return (
                <Pressable
                  key={`${btn.text}-${index}`}
                  className={`h-11 rounded-xl items-center justify-center px-4 ${
                    isRowLayout ? "flex-1" : "w-full"
                  } ${
                    isPrimary
                      ? "bg-foreground"
                      : isCancel
                        ? "bg-[#222327] border border-white/10"
                        : "bg-red-500/15 border border-red-500/30"
                  }`}
                  style={({ pressed }) => ({ opacity: pressed ? 0.75 : 1 })}
                  onPress={() => handleClose(btn.onPress)}
                >
                  <Text
                    className={`text-sm ${
                      isPrimary
                        ? "text-black font-semibold"
                        : isCancel
                          ? "text-[#d4d4d8] font-medium"
                          : "text-red-400 font-semibold"
                    }`}
                  >
                    {btn.text}
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </Animated.View>
      </Pressable>
    </Modal>
  );
}
