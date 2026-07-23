import React, { useState, useEffect } from "react";
import { registerRootComponent } from "expo";
import { StatusBar } from "expo-status-bar";
import {
  StyleSheet,
  Text,
  View,
  TextInput,
  TouchableOpacity,
  FlatList,
  Modal,
  SafeAreaView,
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  Alert,
} from "react-native";
import { Folder02Icon } from "hugeicons-react";
import AsyncStorage from "@react-native-async-storage/async-storage";
import { ConsoleApiProvider, useProjects, useSessions, useCreateSession, useDeleteSession, useAddProject, configureConsoleApi } from "@console/api";
import { QueryClient } from "@tanstack/react-query";

const queryClient = new QueryClient();
const BACKEND_URL_KEY = "@console_backend_url";

function MainApp() {
  const [backendUrl, setBackendUrl] = useState<string | null>(null);
  const [inputUrl, setInputUrl] = useState("http://localhost:3000");
  const [showConfigModal, setShowConfigModal] = useState(false);
  const [loading, setLoading] = useState(true);

  // Tab State: 'home' | 'chat'
  const [activeTab, setActiveTab] = useState<"home" | "chat">("home");
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null);
  const [selectedSessionId, setSelectedSessionId] = useState<string | null>(null);

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
      // Always show dialog/modal on first run or start so user can verify/change it
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

      {/* Main Content Area */}
      {backendUrl ? (
        <ConsoleApiProvider baseUrl={backendUrl} queryClient={queryClient}>
          <SafeAreaView style={styles.safeArea}>
            {activeTab === "home" ? (
              <HomeScreen
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
                  onPress={() => setShowConfigModal(false)}
                >
                  <Text style={styles.btnText}>Cancel</Text>
                </TouchableOpacity>
              )}
              <TouchableOpacity
                style={[styles.btn, styles.btnSave]}
                onPress={saveBackendUrl}
              >
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
          <Text style={[styles.tabText, activeTab === "home" && styles.tabTextActive]}>
            Home
          </Text>
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
          <Text style={[styles.tabText, activeTab === "chat" && styles.tabTextActive]}>
            Chat
          </Text>
        </TouchableOpacity>
        <TouchableOpacity
          style={styles.tabButton}
          onPress={() => setShowConfigModal(true)}
        >
          <Text style={styles.tabTextSettings}>Config</Text>
        </TouchableOpacity>
      </View>
    </View>
  );
}

// HomeScreen Component
function HomeScreen({
  selectedProjectId,
  setSelectedProjectId,
  selectedSessionId,
  setSelectedSessionId,
  setActiveTab,
}: any) {
  const { data: projects = [], refetch: refetchProjects } = useProjects();
  const [projectPathInput, setProjectPathInput] = useState("");
  const addProjectMutation = useAddProject();

  const handleAddProject = async () => {
    if (!projectPathInput.trim()) return;
    try {
      await addProjectMutation.mutateAsync(projectPathInput.trim());
      setProjectPathInput("");
      refetchProjects();
    } catch (e) {
      Alert.alert("Error", "Failed to add project path");
    }
  };

  return (
    <View style={styles.container}>
      <Text style={styles.headerTitle}>Console Projects</Text>

      {/* Add Project Bar */}
      <View style={styles.addProjectBar}>
        <TextInput
          style={styles.projectInput}
          value={projectPathInput}
          onChangeText={setProjectPathInput}
          placeholder="/absolute/path/to/project"
          placeholderTextColor="#9095a0"
          autoCapitalize="none"
          autoCorrect={false}
        />
        <TouchableOpacity style={styles.projectAddBtn} onPress={handleAddProject}>
          <Text style={styles.projectAddBtnText}>Add</Text>
        </TouchableOpacity>
      </View>

      <FlatList
        data={projects}
        keyExtractor={(item) => item.id}
        renderItem={({ item }) => {
          const isSelected = selectedProjectId === item.id;
          return (
            <View style={styles.projectCard}>
              <TouchableOpacity
                style={[styles.projectHeader, isSelected && styles.projectHeaderActive]}
                onPress={() => {
                  setSelectedProjectId(isSelected ? null : item.id);
                  setSelectedSessionId(null);
                }}
              >
                <Folder02Icon size={16} color={isSelected ? "#38bdf8" : "#9095a0"} />
                <Text style={[styles.projectTitle, isSelected && styles.projectTitleActive]}>
                  {item.name}
                </Text>
              </TouchableOpacity>

              {isSelected && (
                <SessionSubList
                  projectId={item.id}
                  projectPath={item.path}
                  selectedSessionId={selectedSessionId}
                  setSelectedSessionId={setSelectedSessionId}
                  setActiveTab={setActiveTab}
                />
              )}
            </View>
          );
        }}
        ListEmptyComponent={
          <View style={styles.emptyList}>
            <Text style={styles.emptyListText}>No projects configured.</Text>
          </View>
        }
      />
    </View>
  );
}

// SessionSubList Component
function SessionSubList({
  projectId,
  projectPath,
  selectedSessionId,
  setSelectedSessionId,
  setActiveTab,
}: any) {
  const { data: sessions = [], refetch: refetchSessions } = useSessions({ projectId });
  const createSessionMutation = useCreateSession();
  const deleteSessionMutation = useDeleteSession();

  const handleCreateSession = async () => {
    try {
      const sess = await createSessionMutation.mutateAsync({
        cwd: projectPath,
        projectId,
        title: "New mobile session",
      });
      setSelectedSessionId(sess.id);
      refetchSessions();
      setActiveTab("chat");
    } catch (e) {
      Alert.alert("Error", "Failed to create session");
    }
  };

  const handleDeleteSession = async (id: string) => {
    Alert.alert("Delete", "Delete this chat session?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Delete",
        style: "destructive",
        onPress: async () => {
          try {
            await deleteSessionMutation.mutateAsync(id);
            if (selectedSessionId === id) {
              setSelectedSessionId(null);
            }
            refetchSessions();
          } catch (e) {
            Alert.alert("Error", "Failed to delete session");
          }
        },
      },
    ]);
  };

  return (
    <View style={styles.sessionsContainer}>
      <View style={styles.sessionHeaderRow}>
        <Text style={styles.sessionsTitle}>Sessions</Text>
        <TouchableOpacity style={styles.newSessionBtn} onPress={handleCreateSession}>
          <Text style={styles.newSessionBtnText}>+ New Chat</Text>
        </TouchableOpacity>
      </View>

      {sessions.map((sess) => {
        const isActive = selectedSessionId === sess.id;
        return (
          <View key={sess.id} style={[styles.sessionRow, isActive && styles.sessionRowActive]}>
            <TouchableOpacity
              style={styles.sessionClickArea}
              onPress={() => {
                setSelectedSessionId(sess.id);
                setActiveTab("chat");
              }}
            >
              <Text style={[styles.sessionText, isActive && styles.sessionTextActive]}>
                {sess.title || "New Chat"}
              </Text>
            </TouchableOpacity>
            <TouchableOpacity
              style={styles.sessionDeleteBtn}
              onPress={() => handleDeleteSession(sess.id)}
            >
              <Text style={styles.sessionDeleteText}>✕</Text>
            </TouchableOpacity>
          </View>
        );
      })}

      {sessions.length === 0 && (
        <Text style={styles.emptySessionsText}>No active chat sessions.</Text>
      )}
    </View>
  );
}

// ChatScreen Component
function ChatScreen({ projectId, sessionId, backendUrl }: { projectId: string | null; sessionId: string | null; backendUrl: string }) {
  const [messages, setMessages] = useState<any[]>([]);
  const [inputVal, setInputVal] = useState("");
  const [running, setRunning] = useState(false);
  const [activeSession, setActiveSession] = useState<string | null>(sessionId);

  useEffect(() => {
    setActiveSession(sessionId);
    if (sessionId) {
      fetchSessionMessages();
    }
  }, [sessionId]);

  const fetchSessionMessages = async () => {
    try {
      const response = await fetch(`${backendUrl}/api/sessions/${sessionId}`);
      const data = await response.json();
      if (data && data.messages) {
        setMessages(data.messages);
      }
    } catch (e) {
      // Ignore initial load fetch errors
    }
  };

  const handleSend = async () => {
    if (!inputVal.trim() || !activeSession) return;
    const prompt = inputVal.trim();
    setInputVal("");
    setRunning(true);

    // Optimistically push user message to UI
    setMessages((prev) => [...prev, { role: "user", content: prompt }]);

    try {
      const res = await fetch(`${backendUrl}/api/sessions/${activeSession}/run`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt }),
      });

      if (!res.body) {
        setRunning(false);
        fetchSessionMessages();
        return;
      }

      // Simple response parsing for SSE fallback
      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let accumulatedText = "";
      let buffer = "";

      // Add temporary response placeholder
      setMessages((prev) => [...prev, { role: "assistant", content: [{ type: "text", text: "" }] }]);

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";

        for (const line of lines) {
          const trimmed = line.trim();
          if (trimmed.startsWith("data: ")) {
            try {
              const frame = JSON.parse(trimmed.slice(6));
              if (frame.type === "modelStreamPart" && frame.part?.text) {
                accumulatedText += frame.part.text;
                setMessages((prev) => {
                  const updated = [...prev];
                  const lastIndex = updated.length - 1;
                  if (updated[lastIndex] && updated[lastIndex].role === "assistant") {
                    updated[lastIndex] = {
                      role: "assistant",
                      content: [{ type: "text", text: accumulatedText }],
                    };
                  }
                  return updated;
                });
              }
            } catch (err) {
              // Ignore frames parse issues
            }
          }
        }
      }
    } catch (e) {
      // Fetch fallback updates
      fetchSessionMessages();
    } finally {
      setRunning(false);
      fetchSessionMessages();
    }
  };

  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      keyboardVerticalOffset={Platform.OS === "ios" ? 90 : 0}
      style={styles.chatContainer}
    >
      <Text style={styles.headerTitle}>Console Chat</Text>

      <FlatList
        data={messages}
        keyExtractor={(_, index) => index.toString()}
        style={styles.messageList}
        contentContainerStyle={styles.messageContentList}
        renderItem={({ item }) => {
          const isUser = item.role === "user";
          let text = "";
          if (typeof item.content === "string") {
            text = item.content;
          } else if (Array.isArray(item.content)) {
            text = item.content
              .map((c: any) => c.text || "")
              .join("\n");
          }

          return (
            <View style={[styles.bubbleContainer, isUser ? styles.bubbleUser : styles.bubbleAgent]}>
              <Text style={styles.bubbleRole}>{isUser ? "You" : "Agent"}</Text>
              <Text style={styles.bubbleText}>{text}</Text>
            </View>
          );
        }}
        ListEmptyComponent={
          <View style={styles.emptyList}>
            <Text style={styles.emptyListText}>No messages. Type a prompt below to start.</Text>
          </View>
        }
      />

      <View style={styles.chatComposer}>
        <TextInput
          style={styles.chatInput}
          value={inputVal}
          onChangeText={setInputVal}
          placeholder="Ask agent to write code..."
          placeholderTextColor="#9095a0"
          multiline
        />
        <TouchableOpacity
          style={[styles.chatSendBtn, (!inputVal.trim() || running) && styles.chatSendBtnDisabled]}
          onPress={handleSend}
          disabled={!inputVal.trim() || running}
        >
          {running ? (
            <ActivityIndicator size="small" color="#09090b" />
          ) : (
            <Text style={styles.chatSendBtnText}>Send</Text>
          )}
        </TouchableOpacity>
      </View>
    </KeyboardAvoidingView>
  );
}

export default registerRootComponent(MainApp);

const styles = StyleSheet.create({
  loadingContainer: {
    flex: 1,
    backgroundColor: "#0d0d0e",
    alignItems: "center",
    justifyContent: "center",
  },
  appContainer: {
    flex: 1,
    backgroundColor: "#0d0d0e",
  },
  safeArea: {
    flex: 1,
    backgroundColor: "#0d0d0e",
  },
  container: {
    flex: 1,
    paddingHorizontal: 16,
    paddingTop: 12,
  },
  headerTitle: {
    fontSize: 20,
    fontWeight: "bold",
    color: "#f1f3f7",
    marginBottom: 16,
    letterSpacing: -0.5,
  },
  emptyContainer: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    padding: 24,
  },
  emptyText: {
    color: "#9095a0",
    textAlign: "center",
    fontSize: 14,
  },
  // Projects list
  addProjectBar: {
    flexDirection: "row",
    gap: 8,
    marginBottom: 16,
  },
  projectInput: {
    flex: 1,
    height: 40,
    backgroundColor: "#16171a",
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.08)",
    borderRadius: 8,
    paddingHorizontal: 12,
    color: "#f1f3f7",
    fontSize: 13,
  },
  projectAddBtn: {
    height: 40,
    backgroundColor: "#f1f3f7",
    borderRadius: 8,
    paddingHorizontal: 16,
    alignItems: "center",
    justifyContent: "center",
  },
  projectAddBtnText: {
    color: "#09090b",
    fontSize: 13,
    fontWeight: "600",
  },
  projectCard: {
    backgroundColor: "#121316",
    borderRadius: 8,
    marginBottom: 8,
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.05)",
    overflow: "hidden",
  },
  projectHeader: {
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
    padding: 14,
  },
  projectHeaderActive: {
    backgroundColor: "rgba(255,255,255,0.04)",
  },
  projectTitle: {
    color: "#9095a0",
    fontSize: 14,
    fontWeight: "500",
  },
  projectTitleActive: {
    color: "#f1f3f7",
  },
  // Sessions sub-list
  sessionsContainer: {
    paddingHorizontal: 14,
    paddingBottom: 14,
    paddingTop: 4,
    borderTopWidth: 1,
    borderTopColor: "rgba(255,255,255,0.04)",
  },
  sessionHeaderRow: {
    flexDirection: "row",
    justifyContent: "between",
    alignItems: "center",
    marginBottom: 8,
    marginTop: 4,
  },
  sessionsTitle: {
    fontSize: 11,
    fontWeight: "700",
    color: "#9095a0",
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  newSessionBtn: {
    paddingVertical: 4,
    paddingHorizontal: 8,
    borderRadius: 4,
    backgroundColor: "rgba(255,255,255,0.06)",
  },
  newSessionBtnText: {
    color: "#f1f3f7",
    fontSize: 10,
    fontWeight: "600",
  },
  sessionRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "between",
    paddingVertical: 8,
    paddingHorizontal: 10,
    borderRadius: 6,
    marginBottom: 4,
    backgroundColor: "rgba(255,255,255,0.02)",
  },
  sessionRowActive: {
    backgroundColor: "rgba(255,255,255,0.08)",
  },
  sessionClickArea: {
    flex: 1,
    paddingRight: 10,
  },
  sessionText: {
    color: "#9095a0",
    fontSize: 12,
  },
  sessionTextActive: {
    color: "#f1f3f7",
    fontWeight: "500",
  },
  sessionDeleteBtn: {
    padding: 4,
  },
  sessionDeleteText: {
    color: "#ef4444",
    fontSize: 11,
  },
  emptySessionsText: {
    fontSize: 11,
    color: "#9095a0",
    fontStyle: "italic",
    textAlign: "center",
    paddingVertical: 8,
  },
  // Modal layout
  modalOverlay: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.85)",
    justifyContent: "center",
    padding: 24,
  },
  modalContent: {
    backgroundColor: "#121316",
    borderRadius: 12,
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.08)",
    padding: 20,
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 10 },
    shadowOpacity: 0.5,
    shadowRadius: 10,
    elevation: 10,
  },
  modalTitle: {
    fontSize: 18,
    fontWeight: "bold",
    color: "#f1f3f7",
    marginBottom: 6,
  },
  modalSub: {
    fontSize: 13,
    color: "#9095a0",
    marginBottom: 16,
  },
  input: {
    height: 44,
    backgroundColor: "#16171a",
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.08)",
    borderRadius: 8,
    paddingHorizontal: 12,
    color: "#f1f3f7",
    fontSize: 14,
    marginBottom: 20,
  },
  modalButtons: {
    flexDirection: "row",
    justifyContent: "flex-end",
    gap: 12,
  },
  btn: {
    paddingVertical: 10,
    paddingHorizontal: 16,
    borderRadius: 8,
    alignItems: "center",
    justifyContent: "center",
    minWidth: 80,
  },
  btnCancel: {
    backgroundColor: "rgba(255,255,255,0.05)",
  },
  btnSave: {
    backgroundColor: "#f1f3f7",
  },
  btnText: {
    fontSize: 13,
    fontWeight: "600",
    color: "#09090b",
  },
  // Bottom tab bar
  bottomBar: {
    height: 60,
    flexDirection: "row",
    borderTopWidth: 1,
    borderTopColor: "rgba(255,255,255,0.06)",
    backgroundColor: "#080809",
    paddingBottom: Platform.OS === "ios" ? 10 : 0,
  },
  tabButton: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
  },
  tabButtonActive: {
    backgroundColor: "rgba(255,255,255,0.02)",
  },
  tabText: {
    color: "#9095a0",
    fontSize: 12,
    fontWeight: "500",
  },
  tabTextActive: {
    color: "#38bdf8",
  },
  tabTextSettings: {
    color: "#9095a0",
    fontSize: 12,
    fontWeight: "500",
  },
  // Chat viewport
  chatContainer: {
    flex: 1,
    paddingTop: 12,
  },
  messageList: {
    flex: 1,
  },
  messageContentList: {
    paddingHorizontal: 16,
    paddingBottom: 80,
  },
  bubbleContainer: {
    padding: 12,
    borderRadius: 10,
    marginBottom: 12,
    maxWidth: "85%",
  },
  bubbleUser: {
    backgroundColor: "rgba(255,255,255,0.05)",
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.08)",
    alignSelf: "flex-end",
  },
  bubbleAgent: {
    backgroundColor: "#121316",
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.04)",
    alignSelf: "flex-start",
    width: "100%",
    maxWidth: "90%",
  },
  bubbleRole: {
    fontSize: 10,
    fontWeight: "700",
    color: "#9095a0",
    textTransform: "uppercase",
    marginBottom: 4,
  },
  bubbleText: {
    color: "#f1f3f7",
    fontSize: 13,
    lineHeight: 18,
  },
  chatComposer: {
    flexDirection: "row",
    gap: 8,
    padding: 12,
    backgroundColor: "#0d0d0e",
    borderTopWidth: 1,
    borderTopColor: "rgba(255,255,255,0.06)",
    alignItems: "center",
  },
  chatInput: {
    flex: 1,
    minHeight: 40,
    maxHeight: 100,
    backgroundColor: "#16171a",
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.08)",
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 8,
    color: "#f1f3f7",
    fontSize: 13,
  },
  chatSendBtn: {
    height: 40,
    backgroundColor: "#f1f3f7",
    borderRadius: 8,
    paddingHorizontal: 16,
    alignItems: "center",
    justifyContent: "center",
  },
  chatSendBtnDisabled: {
    opacity: 0.3,
  },
  chatSendBtnText: {
    color: "#09090b",
    fontSize: 13,
    fontWeight: "600",
  },
  emptyList: {
    alignItems: "center",
    justifyContent: "center",
    paddingVertical: 40,
  },
  emptyListText: {
    color: "#9095a0",
    fontSize: 12,
    fontStyle: "italic",
  },
});
