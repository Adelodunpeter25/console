import React, { useCallback, useRef, useState, useEffect } from "react";
import {
  Modal,
  View,
  Text,
  TextInput,
  Pressable,
  Animated,
  KeyboardAvoidingView,
  Platform,
  ActivityIndicator,
} from "react-native";
import { observable } from "@legendapp/state";
import { useValue } from "@legendapp/state/react";
import { useUpdateSession } from "@/hooks/queries";
import { confirmAlert } from "./confirm-dialog";

export interface RenameDialogOptions {
  sessionId: string;
  currentTitle: string;
  onSuccess?: (newTitle: string) => void;
}

export const renameSessionDialog$ = observable({
  isOpen: false,
  options: null as RenameDialogOptions | null,
});

export function openRenameDialog(options: RenameDialogOptions): void {
  renameSessionDialog$.options.set(options);
  renameSessionDialog$.isOpen.set(true);
}

export function hideRenameDialog(): void {
  renameSessionDialog$.isOpen.set(false);
  renameSessionDialog$.options.set(null);
}

export function RenameSessionDialog() {
  const isOpen = useValue(renameSessionDialog$.isOpen);
  const options = useValue(renameSessionDialog$.options);
  const [title, setTitle] = useState("");
  const updateSessionMutation = useUpdateSession();

  const opacity = useRef(new Animated.Value(0)).current;
  const scale = useRef(new Animated.Value(0.94)).current;
  const inputRef = useRef<TextInput>(null);

  useEffect(() => {
    if (options) {
      setTitle(options.currentTitle || "");
    }
  }, [options]);

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

  const handleClose = useCallback(() => {
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
      hideRenameDialog();
    });
  }, [opacity, scale]);

  const handleSave = useCallback(async () => {
    if (!options) return;
    const trimmed = title.trim();
    if (!trimmed || trimmed === options.currentTitle) {
      handleClose();
      return;
    }

    try {
      await updateSessionMutation.mutateAsync({
        id: options.sessionId,
        payload: { title: trimmed },
      });
      options.onSuccess?.(trimmed);
      handleClose();
    } catch (err) {
      confirmAlert(
        "Failed to Rename",
        err instanceof Error ? err.message : "Unable to update session title.",
      );
    }
  }, [options, title, updateSessionMutation, handleClose]);

  if (!isOpen || !options) {
    return null;
  }

  const isPending = updateSessionMutation.isPending;

  return (
    <Modal
      visible={isOpen}
      transparent
      animationType="none"
      statusBarTranslucent
      onShow={onShow}
      onRequestClose={handleClose}
    >
      <KeyboardAvoidingView
        behavior={Platform.OS === "ios" ? "padding" : "height"}
        className="flex-1"
      >
        <Pressable
          className="flex-1 bg-black/65 justify-center items-center px-6"
          onPress={handleClose}
        >
          <Animated.View
            style={{ opacity, transform: [{ scale }] }}
            className="w-full max-w-[330px] bg-[#16171a] rounded-[20px] border border-white/10 p-5 shadow-2xl"
            onStartShouldSetResponder={() => true}
          >
            <View className="mb-4">
              <Text className="text-[17px] font-semibold text-foreground text-center tracking-tight">
                Rename Chat
              </Text>
              <Text className="text-xs text-foreground-secondary text-center mt-1">
                Enter a new name for this session
              </Text>
            </View>

            <View className="mb-5">
              <TextInput
                ref={inputRef}
                value={title}
                onChangeText={setTitle}
                placeholder="Session title"
                placeholderTextColor="#71717a"
                autoFocus
                selectTextOnFocus
                returnKeyType="done"
                onSubmitEditing={handleSave}
                editable={!isPending}
                className="w-full h-11 bg-[#222327] border border-white/10 rounded-xl px-3.5 text-sm text-foreground"
              />
            </View>

            <View className="flex-row gap-2.5">
              <Pressable
                className="flex-1 h-11 rounded-xl items-center justify-center px-4 bg-[#222327] border border-white/10"
                style={({ pressed }) => ({ opacity: pressed ? 0.75 : 1 })}
                onPress={handleClose}
                disabled={isPending}
              >
                <Text className="text-sm text-[#d4d4d8] font-medium">Cancel</Text>
              </Pressable>

              <Pressable
                className="flex-1 h-11 rounded-xl items-center justify-center px-4 bg-foreground"
                style={({ pressed }) => ({ opacity: pressed || !title.trim() ? 0.75 : 1 })}
                onPress={handleSave}
                disabled={isPending || !title.trim()}
              >
                {isPending ? (
                  <ActivityIndicator size="small" color="#000000" />
                ) : (
                  <Text className="text-sm text-black font-semibold">Save</Text>
                )}
              </Pressable>
            </View>
          </Animated.View>
        </Pressable>
      </KeyboardAvoidingView>
    </Modal>
  );
}
