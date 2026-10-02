// Device simulator wire types (desktop contract for /api/devices).
package types

type DeviceDescriptor struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Platform    string  `json:"platform"`
	State       string  `json:"state"` // booted | shutdown | booting
	Model       *string `json:"model"`
	OSVersion   *string `json:"osVersion"`
	IsAvailable bool    `json:"isAvailable"`
}

type DeviceDiagnostics struct {
	XcodeInstalled     bool     `json:"xcodeInstalled"`
	XcodeVersion       *string  `json:"xcodeVersion"`
	SimctlAvailable    bool     `json:"simctlAvailable"`
	AndroidSDKFound    bool     `json:"androidSdkFound"`
	ADBAvailable       bool     `json:"adbAvailable"`
	EmulatorAvailable  bool     `json:"emulatorAvailable"`
	DiskFreeBytes      uint64   `json:"diskFreeBytes"`
	HasEnoughDiskSpace bool     `json:"hasEnoughDiskSpace"`
	Errors             []string `json:"errors"`
}

// DeviceAction is a one-shot REST interaction. Live touch input goes over
// the stream socket instead; tap/swipe coordinates here are device pixels.
type DeviceAction struct {
	Action     string   `json:"action"`
	X          *float64 `json:"x"`
	Y          *float64 `json:"y"`
	EndX       *float64 `json:"endX"`
	EndY       *float64 `json:"endY"`
	DurationMs *int     `json:"durationMs"`
	Text       string   `json:"text"`
	Key        string   `json:"key"`
	Appearance string   `json:"appearance"`
}

type DeviceOpenAppRequest struct {
	App string `json:"app"`
}

// DeviceStreamMeta is the first text frame on a device stream socket.
type DeviceStreamMeta struct {
	Type   string `json:"type"` // always "meta"
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Name   string `json:"name"`
	Codec  string `json:"codec"`
}

// Binary stream frame tags (first byte of every binary frame).
const (
	DeviceFrameDescription byte = 1 // avcC decoder configuration
	DeviceFrameKey         byte = 2 // AVCC key frame
	DeviceFrameDelta       byte = 3 // AVCC delta frame
)
