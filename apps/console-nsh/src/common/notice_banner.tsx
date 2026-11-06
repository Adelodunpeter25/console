// The agent notice banner, ported from console-ui's `common/notice_banner.rs`:
// a neutral strip for transient status notices such as a context-compaction
// summary.

import { Text, View } from "nsh";
import { CenteredStripe } from "./stripe";

export function NoticeBanner(props: { message: string }) {
  return (
    <CenteredStripe paddingY={6}>
      <View class="w-full max-w-[768px] rounded-md bg-muted px-2.5 py-1.5">
        <Text class="text-[11.5px] text-muted-foreground">{props.message}</Text>
      </View>
    </CenteredStripe>
  );
}
