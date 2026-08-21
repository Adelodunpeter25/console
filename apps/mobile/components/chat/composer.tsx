import React, { useState } from "react";
import { View, TextInput, Pressable, StyleSheet, Image, ActivityIndicator } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { KeyboardStickyView, useKeyboardState } from "react-native-keyboard-controller";
import * as ImagePicker from "expo-image-picker";
import { ArrowUp, Square, Plus, X } from "lucide-react-native";
import type { ImageAttachment } from "@console/types";
import { theme } from "../../styles/theme";
import { useAppStore, useChatStore, useProjectStore, useSessionStore } from "../../stores";
import { ImagePreviewModal } from "../common/image-preview-modal";
import { ComposerBottomStrip } from "./composer-bottom-strip";

interface ComposerProps {
  value: string;
  onChangeText: (text: string) => void;
  onSend: () => void;
  onStop?: () => void;
  running?: boolean;
  isAborting?: boolean;
  projectLocked?: boolean;
}

/** Chat input row matching unified pill with left attachment button and bottom selectors strip. */
export function Composer({
  value,
  onChangeText,
  onSend,
  onStop,
  running,
  isAborting,
  projectLocked,
}: ComposerProps) {
  const insets = useSafeAreaInsets();
  const keyboardVisible = useKeyboardState((s) => s.isVisible);
  const paddingBottom = keyboardVisible ? 8 : Math.max(insets.bottom, 8) + 4;

  const selectedSessionId = useAppStore((state) => state.selectedSessionId);
  const projects = useProjectStore((state) => state.projects);
  const sessionView = useSessionStore((state) =>
    selectedSessionId ? state.getSession(selectedSessionId) : undefined,
  );
  const changeModel = useSessionStore((state) => state.changeModel);
  const changeProject = useSessionStore((state) => state.changeProject);
  const setApprovalMode = useSessionStore((state) => state.setApprovalMode);

  const attachments = useChatStore((state) =>
    selectedSessionId ? state.getSession(selectedSessionId).attachments : [],
  );
  const addAttachments = useChatStore((state) => state.addAttachments);
  const removeAttachment = useChatStore((state) => state.removeAttachment);

  const selectedProject = projects.find(
    (p) =>
      p.path === sessionView?.sessionCwd ||
      (sessionView?.sessionCwd && p.path.endsWith(sessionView.sessionCwd)),
  );

  const [isMultiline, setIsMultiline] = useState(false);
  const [previewImageUri, setPreviewImageUri] = useState<string | null>(null);
  const canSend = value.trim().length > 0 || attachments.length > 0;

  const handlePickImages = async () => {
    if (!selectedSessionId) return;
    try {
      if (!ImagePicker?.launchImageLibraryAsync) {
        console.warn("ImagePicker native module not found in binary");
        return;
      }
      const result = await ImagePicker.launchImageLibraryAsync({
        mediaTypes: ["images"],
        allowsMultipleSelection: true,
        base64: true,
        quality: 0.85,
      });

      if (!result.canceled && result.assets && result.assets.length > 0) {
        const newAttachments: ImageAttachment[] = result.assets
          .filter((asset) => Boolean(asset.base64))
          .map((asset) => ({
            data: asset.base64!,
            mimeType: asset.mimeType ?? "image/jpeg",
          }));

        if (newAttachments.length > 0) {
          addAttachments(selectedSessionId, newAttachments);
        }
      }
    } catch (error) {
      console.warn("Failed to pick images from file explorer:", error);
    }
  };

  return (
    <>
      <KeyboardStickyView offset={{ closed: 0, opened: 0 }}>
        <View className="px-2.5 pt-2 bg-screen" style={{ paddingBottom }}>
          {/* Attachment Previews Strip */}
          {attachments.length > 0 ? (
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
                      <Image
                        source={{ uri: imageUri }}
                        className="w-14 h-14"
                        resizeMode="cover"
                      />
                    </Pressable>
                    <Pressable
                      onPress={() => removeAttachment(selectedSessionId!, idx)}
                      className="absolute top-1 right-1 w-5 h-5 rounded-full bg-black/80 items-center justify-center border border-white/20"
                      hitSlop={6}
                    >
                      <X size={11} color="#ffffff" />
                    </Pressable>
                  </View>
                );
              })}
            </View>
          ) : null}

          <View
            className={`flex-row ${
              isMultiline ? "items-end pb-1" : "items-center"
            } bg-card border border-border/80 pl-2 pr-1.5 py-1 min-h-[48px] ${
              isMultiline || attachments.length > 0 ? "rounded-2xl" : "rounded-full"
            }`}
          >
            {/* Left Side Attach Button (+) */}
            <Pressable
              onPress={handlePickImages}
              className="w-8 h-8 rounded-full items-center justify-center mr-1"
              style={({ pressed }) => ({
                backgroundColor: pressed ? "rgba(255,255,255,0.08)" : "transparent",
                opacity: pressed ? 0.7 : 0.9,
              })}
              hitSlop={6}
            >
              <Plus size={21} color={theme.colors.text.secondary} />
            </Pressable>

            <TextInput
              style={styles.input}
              value={value}
              onChangeText={onChangeText}
              placeholder="Ask the repo agent, or run a command…"
              placeholderTextColor={theme.colors.text.muted}
              multiline
              textAlignVertical={isMultiline ? "top" : "center"}
              onContentSizeChange={(e) => {
                const height = e.nativeEvent.contentSize.height;
                setIsMultiline(height > 36 || value.includes("\n"));
              }}
            />

            {running && onStop ? (
              <Pressable
                className="w-8 h-8 rounded-full items-center justify-center ml-1.5"
                style={({ pressed }) => ({
                  backgroundColor: theme.colors.text.primary,
                  opacity: pressed ? 0.7 : 1,
                })}
                onPress={onStop}
                accessibilityLabel="Stop"
              >
                <Square size={12} color="#000000" fill="#000000" />
              </Pressable>
            ) : (
              <Pressable
                className="w-8 h-8 rounded-full items-center justify-center ml-1.5"
                style={({ pressed }) => ({
                  backgroundColor: canSend ? theme.colors.text.primary : "rgba(255,255,255,0.08)",
                  opacity: pressed && canSend ? 0.8 : 1,
                })}
                onPress={onSend}
                disabled={!canSend}
              >
                <ArrowUp
                  size={16}
                  color={canSend ? theme.colors.text.dark : theme.colors.text.muted}
                />
              </Pressable>
            )}
          </View>

          {/* Bottom Selector Strip: Project, Model, Approval Mode */}
          {selectedSessionId ? (
            <ComposerBottomStrip
              projects={projects}
              selectedProjectId={selectedProject?.id ?? null}
              onProjectChange={(project) => changeProject(selectedSessionId, project)}
              projectLocked={projectLocked}
              selectedModel={sessionView?.sessionModelId ?? null}
              selectedProvider={sessionView?.sessionProvider ?? null}
              onModelChange={(modelId) => changeModel(selectedSessionId, modelId)}
              approvalMode={sessionView?.approvalMode ?? "always-ask"}
              onApprovalModeChange={(mode) => setApprovalMode(selectedSessionId, mode)}
            />
          ) : null}
        </View>
      </KeyboardStickyView>

      {/* Full screen Image Preview Modal */}
      <ImagePreviewModal
        visible={Boolean(previewImageUri)}
        imageUri={previewImageUri}
        onClose={() => setPreviewImageUri(null)}
      />
    </>
  );
}

const styles = StyleSheet.create({
  input: {
    flex: 1,
    maxHeight: 120,
    paddingTop: 8,
    paddingBottom: 8,
    color: theme.colors.text.primary,
    fontSize: 14,
    lineHeight: 19,
    includeFontPadding: false,
  },
});
