import React from "react";
import { View, Text, TouchableOpacity } from "react-native";

interface ErrorBoundaryProps {
  children: React.ReactNode;
}

interface ErrorBoundaryState {
  hasError: boolean;
  message?: string;
}

/**
 * Catches render-time errors in the subtree and shows a recovery screen
 * instead of white-screening the whole app.
 */
export class ErrorBoundary extends React.Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { hasError: false };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, message: error.message };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error("ErrorBoundary caught:", error, info.componentStack);
  }

  private handleReset = () => {
    this.setState({ hasError: false, message: undefined });
  };

  render() {
    if (!this.state.hasError) return this.props.children;

    return (
      <View className="flex-1 bg-screen items-center justify-center p-8">
        <Text className="text-foreground text-lg font-bold mb-2">Something went wrong</Text>
        <Text className="text-foreground-secondary text-xs text-center mb-6 leading-5" numberOfLines={4}>
          {this.state.message || "An unexpected error occurred."}
        </Text>
        <TouchableOpacity
          onPress={this.handleReset}
          className="bg-foreground rounded-xl px-6 py-3 active:opacity-80"
        >
          <Text className="text-sm font-bold text-black">Try Again</Text>
        </TouchableOpacity>
      </View>
    );
  }
}
