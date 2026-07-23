import React from "react";
import {
  Text,
  View,
  TextInput,
  TouchableOpacity,
  Modal,
  KeyboardAvoidingView,
  Platform,
} from "react-native";

interface ConfigModalProps {
  visible: boolean;
  backendUrl: string | null;
  inputUrl: string;
  setInputUrl: (url: string) => void;
  onSave: () => void;
  onClose: () => void;
}

export function ConfigModal({
  visible,
  backendUrl,
  inputUrl,
  setInputUrl,
  onSave,
  onClose,
}: ConfigModalProps) {
  return (
    <Modal
      visible={visible}
      transparent={true}
      animationType="slide"
      onRequestClose={onClose}
    >
      <View className="flex-1 bg-black/85 justify-center p-6">
        <KeyboardAvoidingView
          behavior={Platform.OS === "ios" ? "padding" : "height"}
          className="bg-[#121316] rounded-xl border border-white/10 p-5 shadow-2xl"
        >
          <Text className="text-lg font-bold text-[#f1f3f7] mb-1.5">
            Backend Connection
          </Text>
          <Text className="text-xs text-[#9095a0] mb-4">
            Specify your Console agent server endpoint:
          </Text>
          <TextInput
            className="h-11 bg-[#16171a] border border-white/10 rounded-lg px-3 text-[#f1f3f7] text-sm mb-5"
            value={inputUrl}
            onChangeText={setInputUrl}
            placeholder="http://192.168.1.X:3000"
            placeholderTextColor="#9095a0"
            autoCapitalize="none"
            autoCorrect={false}
          />
          <View className="flex-row justify-end gap-3">
            {backendUrl && (
              <TouchableOpacity
                className="py-2.5 px-4 rounded-lg bg-white/5 items-center justify-center min-w-[80px]"
                onPress={onClose}
              >
                <Text className="text-xs font-semibold text-[#f1f3f7]">Cancel</Text>
              </TouchableOpacity>
            )}
            <TouchableOpacity
              className="py-2.5 px-4 rounded-lg bg-[#f1f3f7] items-center justify-center min-w-[80px]"
              onPress={onSave}
            >
              <Text className="text-xs font-semibold text-[#09090b]">Connect</Text>
            </TouchableOpacity>
          </View>
        </KeyboardAvoidingView>
      </View>
    </Modal>
  );
}
