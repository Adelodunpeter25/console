// Require polyfill synchronously before ES module imports (prevent Babel hoisting of @console/api/axios)
require("./polyfill");
import React, { useState, useEffect } from "react";
import { registerRootComponent } from "expo";
import { StatusBar } from "expo-status-bar";
import {
  Text,
  View,
  TextInput,
  TouchableOpacity,
  Modal,
  SafeAreaView,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  Alert,
} from "react-native";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { ConsoleApiProvider, useProjects, configureConsoleApi } from "@console/api";
import { QueryClient } from "@tanstack/react-query";
import { styles } from "./styles/styles";
import { HomeScreen } from "./components/common/home-screen";
import { ChatScreen } from "./components/chat/chat-screen";

const queryClient = new QueryClient();
const BACKEND_URL_KEY = "@console_backend_url";

function AppRoot() {
  const [backendUrl, setBackendUrl] = useState<string | null>(null);
  const [inputUrl, setInputUrl] = useState("http://localhost:3000");
  const [showConfigModal, setShowConfigModal] = useState(false);
  const [loading, setLoading] = useState(true);

  // Tab State: 'home' | 'chat'
  const [activeTab, setActiveTab] = useState<"home" | "chat">("home");
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null);
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);

  const { data: projects = [], refetch: refetchProjects } = useProjects();

  useEffect(() => {
    loadBackendUrl();
  }, []);

  const loadBackendUrl = async () => {
    try {
      const stored = await AsyncStorage.getItem(BACKEND_URL_KEY);
      if (stored) {
        setBackendUrl(stored);
        setInputUrl(stored);
        configureConsoleApi({ baseUrl: stored });
      }
      setShowConfigModal(true);
    } catch (e) {
      setShowConfigModal(true);
    } finally {
      setLoading(false);
    }
  };

  const saveBackendUrl = async () => {
    if (!inputUrl.trim()) {
      Alert.alert("Error", "Backend URL cannot be empty");
      return;
    }
    try {
      await AsyncStorage.setItem(BACKEND_URL_KEY, inputUrl.trim());
      setBackendUrl(inputUrl.trim());
      configureConsoleApi({ baseUrl: inputUrl.trim() });
      setShowConfigModal(false);
    } catch (e) {
      Alert.alert("Error", "Failed to save URL");
    }
  };

  if (loading) {
    return (
      <View style={styles.loadingContainer}>
        <ActivityIndicator size="large" color="#ffffff" />
      </View>
    );
  }

  return (
    <View style={styles.appContainer}>
      <StatusBar style="light" />

      {backendUrl ? (
        <ConsoleApiProvider baseUrl={backendUrl} queryClient={queryClient}>
          <SafeAreaView style={styles.safeArea}>
            {activeTab === "home" ? (
              <HomeScreen
                projects={projects}
                refetchProjects={refetchProjects}
                selectedProjectId={selectedProjectId}
                setSelectedProjectId={setSelectedProjectId}
                selectedSessionId={selectedSessionId}
                setSelectedSessionId={setSelectedSessionId}
                setActiveTab={setActiveTab}
              />
            ) : (
              <ChatScreen
                projectId={selectedProjectId}
                sessionId={selectedSessionId}
                backendUrl={backendUrl}
              />
            )}
          </SafeAreaView>
        </ConsoleApiProvider>
      ) : (
        <SafeAreaView style={styles.safeArea}>
          <View style={styles.emptyContainer}>
            <Text style={styles.emptyText}>Please configure the backend URL to start.</Text>
          </View>
        </SafeAreaView>
      )}

      {/* Backend Configuration Modal */}
      <Modal
        visible={showConfigModal}
        transparent={true}
        animationType="slide"
        onRequestClose={() => {
          if (backendUrl) setShowConfigModal(false);
        }}
      >
        <View style={styles.modalOverlay}>
          <KeyboardAvoidingView
            behavior={Platform.OS === "ios" ? "padding" : "height"}
            style={styles.modalContent}
          >
            <Text style={styles.modalTitle}>Backend Connection</Text>
            <Text style={styles.modalSub}>Specify your Console agent server endpoint:</Text>
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
                  onPress={() => setShowConfigModal(false)}
                >
                  <Text style={styles.btnText}>Cancel</Text>
                </TouchableOpacity>
              )}
              <TouchableOpacity style={[styles.btn, styles.btnSave]} onPress={saveBackendUrl}>
                <Text style={styles.btnText}>Connect</Text>
              </TouchableOpacity>
            </View>
          </KeyboardAvoidingView>
        </View>
      </Modal>

      {/* Bottom Navigation Bar */}
      <View style={styles.bottomBar}>
        <TouchableOpacity
          style={[styles.tabButton, activeTab === "home" && styles.tabButtonActive]}
          onPress={() => setActiveTab("home")}
        >
          <Text style={[styles.tabText, activeTab === "home" && styles.tabTextActive]}>Home</Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={[styles.tabButton, activeTab === "chat" && styles.tabButtonActive]}
          onPress={() => {
            if (!selectedSessionId) {
              Alert.alert("No Session", "Select or create a chat session on the Home tab first.");
              return;
            }
            setActiveTab("chat");
          }}
        >
          <Text style={[styles.tabText, activeTab === "chat" && styles.tabTextActive]}>Chat</Text>
        </TouchableOpacity>
        <TouchableOpacity style={styles.tabButton} onPress={() => setShowConfigModal(true)}>
          <Text style={styles.tabTextSettings}>Config</Text>
        </TouchableOpacity>
      </View>
    </View>
  );
}

export default registerRootComponent(AppRoot);
