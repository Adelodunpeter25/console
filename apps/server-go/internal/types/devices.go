// Device simulator transport framing.
package types

// Device simulator wire types moved to the shared protobuf schema
// (console.v1 from proto/console/v1/device.proto): DeviceDescriptor,
// DeviceDiagnostics, DeviceActionRequest, DeviceOpenAppRequest/Response and
// DeviceStreamMeta are all generated types now.

// Binary stream frame tags (first byte of every binary frame). Transport
// framing, never a JSON payload — these stay Go constants.
const (
	DeviceFrameDescription byte = 1 // avcC decoder configuration
	DeviceFrameKey         byte = 2 // AVCC key frame
	DeviceFrameDelta       byte = 3 // AVCC delta frame
)