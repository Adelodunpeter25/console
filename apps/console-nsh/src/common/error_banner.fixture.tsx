// The error banner test fixture: `ErrorBanner` over
// `error_banner.fixture.state`, with the dismiss callback rendered on screen
// for the tests to observe.
import { ErrorBanner } from "./error_banner";
import { dismissed } from "./error_banner.fixture.state";
import { Text, View, mount } from "nsh";

function Fixture() {
  return (
    <View class="w-full p-2">
      <ErrorBanner
        message="Session failed to start: connection refused"
        onCopied={() => dismissed.set("yes")}
      />
      <Text>Dismissed: {dismissed() || "no"}</Text>
    </View>
  );
}

mount(Fixture);
