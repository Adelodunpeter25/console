// The pairing gate: until this app holds a device credential for the
// server, the shell shows this card instead of the workspace. Ported from
// the desktop's `state/server_auth.rs` pairing form — host, pairing code
// (shown by the server), device name, submit. The credential lands in the
// shared per-server store the desktop also reads, so one pairing covers
// both apps.

import { signal, Text, View } from "nsh";
import { Input } from "nsh/base";
import { Console } from "console-nsh/native";

export function PairingCard(props: { baseUrl: string; onPaired: () => void }) {
  const host = signal(props.baseUrl);
  const code = signal("");
  const deviceName = signal("Console (nsh)");
  const busy = signal(false);
  const error = signal<string | null>(null);

  const submit = () => {
    if (busy()) return;
    busy.set(true);
    error.set(null);
    void (async () => {
      try {
        await Console.authPairWithCode(host(), code().trim(), deviceName().trim() || "Console (nsh)");
        props.onPaired();
      } catch (err) {
        error.set(err instanceof Error ? err.message : String(err));
      } finally {
        busy.set(false);
      }
    })();
  };

  return (
    <View class="flex-1 items-center justify-center">
      <View class="flex-col gap-3 rounded-xl border border-border bg-card p-5" style={{ width: 420 }}>
        <Text class="text-[14px] font-semibold text-foreground">Pair with Console server</Text>
        <Text class="text-[12px] text-muted-foreground">
          Enter the pairing code shown by your Console server. The credential is stored per
          server and shared with the desktop app.
        </Text>
        <Input
          multiline={false}
          class="px-3 py-2 text-[13px] text-foreground"
          placeholder="Server address (e.g. localhost:2003)"
          value={host()}
          onChange={(e: { value: string }) => host.set(e.value)}
        />
        <Input
          multiline={false}
          class="px-3 py-2 text-[13px] text-foreground"
          placeholder="Pairing code"
          value={code()}
          onChange={(e: { value: string }) => code.set(e.value)}
          onSubmit={submit}
        />
        <Input
          multiline={false}
          class="px-3 py-2 text-[13px] text-foreground"
          placeholder="Device name"
          value={deviceName()}
          onChange={(e: { value: string }) => deviceName.set(e.value)}
          onSubmit={submit}
        />
        {error() != null ? (
          <Text class="text-[12px] text-red-500">{error()}</Text>
        ) : null}
        <View
          class={
            "flex-row items-center justify-center rounded-md px-3 py-2 " +
            (busy() ? "bg-muted" : "bg-primary hover:bg-primary/85")
          }
          role="button"
          accessibilityLabel="Pair"
          onPress={submit}
        >
          <Text class={"text-[13px] font-medium " + (busy() ? "text-muted-foreground" : "text-primary-foreground")}>
            {busy() ? "Pairing…" : "Pair"}
          </Text>
        </View>
      </View>
    </View>
  );
}
