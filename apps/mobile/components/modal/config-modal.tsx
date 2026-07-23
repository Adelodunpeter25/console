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
import { styles } from "../../styles/styles";

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
      <View style={styles.modalOverlay}>
        <KeyboardAvoidingView
          behavior={Platform.OS === "ios" ? "padding" : "height"}
          style={styles.modalContent}
        >
          <Text style={styles.modalTitle}>Backend Connection</Text>
          <Text style={styles.modalSub}>
            Specify your Console agent server endpoint:
          </Text>
          <TextInput
            style={styles.input}
            value={inputUrl}
            onChangeText={setInputUrl}
            placeholder="http://192.168.1.X:3000"
            placeholderTextColor="#9095a0"
            autoCapitalize="none"
            autoCorrect={false}
          />
          <View style={styles.modalButtons}>
            {backendUrl && (
              <TouchableOpacity
                style={[styles.btn, styles.btnCancel]}
                onPress={onClose}
              >
                <Text style={styles.btnText}>Cancel</Text>
              </TouchableOpacity>
            )}
            <TouchableOpacity
              style={[styles.btn, styles.btnSave]}
              onPress={onSave}
            >
              <Text style={styles.btnText}>Connect</Text>
            </TouchableOpacity>
          </View>
        </KeyboardAvoidingView>
      </View>
    </Modal>
  );
}
