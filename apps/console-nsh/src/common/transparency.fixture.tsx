// The transparency checkerboard fixture: the grid alone, and covered by an
// opaque card (an opaque image covers the grid entirely; a transparent one
// leaves it visible).
import { Text, View, mount } from "nsh";
import { TransparencyGrid } from "./transparency";

function Fixture() {
  return (
    <View class="flex-row items-start gap-2 p-2">
      <View class="relative h-[80px] w-[80px] overflow-hidden rounded-lg border border-border">
        <TransparencyGrid />
        <Text>transparent</Text>
      </View>
      <View class="relative h-[80px] w-[80px] overflow-hidden rounded-lg border border-border bg-foreground">
        <TransparencyGrid />
        <Text class="text-background">opaque</Text>
      </View>
    </View>
  );
}

mount(Fixture);
