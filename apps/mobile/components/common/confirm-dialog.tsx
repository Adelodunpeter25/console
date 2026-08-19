import React, { useCallback, useEffect, useRef } from "react";
import {
  Modal,
  View,
  Text,
  Pressable,
  StyleSheet,
  Animated,
  Dimensions,
} from "react-native";
import { create } from "zustand";

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

interface ConfirmDialogState {
  isOpen: boolean;
  options: ConfirmDialogOptions | null;
  show: (options: ConfirmDialogOptions) => void;
  hide: () => void;
}

export const useConfirmDialogStore = create<ConfirmDialogState>((set) => ({
  isOpen: false,
  options: null,
  show: (options) => set({ isOpen: true, options }),
  hide: () => set({ isOpen: false, options: null }),
}));

/**
 * Drop-in replacement for React Native's Alert.alert()
 */
export function confirmAlert(
  title: string,
  message?: string,
  buttons?: ConfirmDialogButton[],
) {
  useConfirmDialogStore.getState().show({
    title,
    message,
    buttons: buttons && buttons.length > 0 ? buttons : [{ text: "OK", style: "default" }],
  });
}

export function ConfirmDialog() {
  const { isOpen, options, hide } = useConfirmDialogStore();
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
        hide();
        if (onPress) {
          setTimeout(() => {
            void onPress();
          }, 50);
        }
      });
    },
    [opacity, scale, hide],
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
      <Pressable style={styles.overlay} onPress={() => handleClose()}>
        <Animated.View
          style={[
            styles.card,
            {
              opacity,
              transform: [{ scale }],
            },
          ]}
          onStartShouldSetResponder={() => true}
        >
          <View style={styles.content}>
            <Text style={styles.title}>{options.title}</Text>
            {options.message ? (
              <Text style={styles.message}>{options.message}</Text>
            ) : null}
          </View>

          <View style={[styles.buttonsContainer, isRowLayout && styles.buttonsRow]}>
            {buttons.map((btn, index) => {
              const isDestructive = btn.style === "destructive";
              const isCancel = btn.style === "cancel";
              const isPrimary = !isDestructive && !isCancel;

              return (
                <Pressable
                  key={`${btn.text}-${index}`}
                  style={({ pressed }) => [
                    styles.button,
                    isRowLayout && styles.buttonFlex,
                    isPrimary && styles.primaryButton,
                    isCancel && styles.cancelButton,
                    isDestructive && styles.destructiveButton,
                    pressed && styles.buttonPressed,
                  ]}
                  onPress={() => handleClose(btn.onPress)}
                >
                  <Text
                    style={[
                      styles.buttonText,
                      isPrimary && styles.primaryButtonText,
                      isCancel && styles.cancelButtonText,
                      isDestructive && styles.destructiveButtonText,
                    ]}
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

const { width: SCREEN_WIDTH } = Dimensions.get("window");

const styles = StyleSheet.create({
  overlay: {
    flex: 1,
    backgroundColor: "rgba(0, 0, 0, 0.65)",
    justifyContent: "center",
    alignItems: "center",
    paddingHorizontal: 24,
  },
  card: {
    width: Math.min(SCREEN_WIDTH - 48, 330),
    backgroundColor: "#16171a",
    borderRadius: 20,
    borderWidth: 1,
    borderColor: "rgba(255, 255, 255, 0.08)",
    padding: 20,
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 12 },
    shadowOpacity: 0.5,
    shadowRadius: 24,
    elevation: 24,
  },
  content: {
    marginBottom: 20,
  },
  title: {
    fontSize: 17,
    fontWeight: "600",
    color: "#ffffff",
    textAlign: "center",
    letterSpacing: -0.2,
  },
  message: {
    fontSize: 14,
    color: "#a1a1aa",
    textAlign: "center",
    marginTop: 8,
    lineHeight: 20,
  },
  buttonsContainer: {
    gap: 8,
  },
  buttonsRow: {
    flexDirection: "row",
    gap: 10,
  },
  button: {
    height: 44,
    borderRadius: 12,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 16,
  },
  buttonFlex: {
    flex: 1,
  },
  buttonPressed: {
    opacity: 0.75,
  },
  primaryButton: {
    backgroundColor: "#ffffff",
  },
  primaryButtonText: {
    color: "#0a0a0b",
    fontSize: 14,
    fontWeight: "600",
  },
  cancelButton: {
    backgroundColor: "#222327",
    borderWidth: 1,
    borderColor: "rgba(255, 255, 255, 0.08)",
  },
  cancelButtonText: {
    color: "#d4d4d8",
    fontSize: 14,
    fontWeight: "500",
  },
  destructiveButton: {
    backgroundColor: "rgba(239, 68, 68, 0.15)",
    borderWidth: 1,
    borderColor: "rgba(239, 68, 68, 0.25)",
  },
  destructiveButtonText: {
    color: "#f87171",
    fontSize: 14,
    fontWeight: "600",
  },
  buttonText: {
    fontSize: 14,
  },
});
