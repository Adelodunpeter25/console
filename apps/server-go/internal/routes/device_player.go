// The device-stream player page served at GET /api/devices/player (see
// registerDeviceRoutes). Client webviews load it from the server origin so
// the page and its `ws://` device stream are always same-origin; a locally
// embedded `data:` page cannot open the upgrade, which left the desktop
// Devices tab reporting "disconnected" even though the stream was healthy.
//
// Wire (identical for iOS and Android), owned by the Go pump:
//
//	server -> player text:    {"type":"meta","width","height","name","codec"}
//	server -> player binary:  tag byte + payload; `1` avcC description,
//	                          `2` key frame, `3` delta (AVCC H.264).
//	player -> server text:    {"type":"touch","action":"down|move|up","x","y"},
//	                          `scroll`, `key` (Android keycode), `text`,
//	                          `button`, `reset`; x/y in stream pixels.
package routes

import _ "embed"

// devicePlayerHTML is the single player page served at /api/devices/player.
//
//go:embed device_player.html
var devicePlayerHTML string
