// Fixture for the notice banner.
import { NoticeBanner } from "../common/notice_banner";
import { mount, View } from "nsh";

function Fixture() {
  return (
    <View class="h-32 w-96">
      <NoticeBanner message="Context compacted — earlier messages summarized" />
    </View>
  );
}

mount(Fixture);
