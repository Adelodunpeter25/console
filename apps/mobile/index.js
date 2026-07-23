// Polyfill FormData before Expo or any monorepo dependency loads
require("./polyfill");

import { registerRootComponent } from "expo";
import AppRoot from "./index";

registerRootComponent(AppRoot);
