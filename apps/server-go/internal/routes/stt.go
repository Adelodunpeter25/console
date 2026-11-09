// Speech-to-text routes (/api/stt). Credentials are configured server-side;
// clients only send audio and receive the transcript.
package routes

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/stt"
)

// RegisterSttRoutes wires the /api/stt routes onto an app. Exported so tests
// can build a Fiber app with only these routes, mirroring the other domains.
func RegisterSttRoutes(app *fiber.App, client *stt.Client) {
	app.Get("/api/stt/status", func(c *fiber.Ctx) error {
		status, err := stt.CurrentStatus()
		if err != nil {
			return sttInternalError(c, err)
		}
		return respondSttStatus(c, status)
	})

	// Both halves are required on every write: the token is never returned to
	// a client, so an account-ID-only update cannot preserve the old one
	// without leaving the UI unable to explain what is stored.
	app.Put("/api/stt/config", func(c *fiber.Ctx) error {
		var req struct {
			AccountID string `json:"accountId"`
			APIToken  string `json:"apiToken"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fail400(c, errors.New("Invalid request body."))
		}
		if err := stt.Set(req.AccountID, req.APIToken); err != nil {
			if errors.Is(err, stt.ErrEmptyAccountID) || errors.Is(err, stt.ErrEmptyAPIToken) {
				return fail400(c, err)
			}
			return sttInternalError(c, err)
		}
		status, err := stt.CurrentStatus()
		if err != nil {
			return sttInternalError(c, err)
		}
		return respondSttConfig(c, status, false)
	})

	app.Delete("/api/stt/config", func(c *fiber.Ctx) error {
		if err := stt.Clear(); err != nil {
			return sttInternalError(c, err)
		}
		status, err := stt.CurrentStatus()
		if err != nil {
			return sttInternalError(c, err)
		}
		return respondSttConfig(c, status, true)
	})

	app.Post("/api/stt/transcribe", func(c *fiber.Ctx) error {
		file, err := c.FormFile("audio")
		if err != nil {
			return fail400(c, errors.New("An audio file is required in the 'audio' field."))
		}
		if file.Size > int64(stt.MaxAudioBytes) {
			return sttAudioTooLarge(c)
		}
		f, err := file.Open()
		if err != nil {
			return fail400(c, errors.New("Could not read the audio file."))
		}
		defer f.Close()
		audio, err := io.ReadAll(io.LimitReader(f, int64(stt.MaxAudioBytes)+1))
		if err != nil {
			return fail400(c, errors.New("Could not read the audio file."))
		}
		if len(audio) > stt.MaxAudioBytes {
			return sttAudioTooLarge(c)
		}

		text, err := client.Transcribe(c.Context(), audio, file.Header.Get("Content-Type"))
		if err != nil {
			switch {
			case errors.Is(err, stt.ErrEmptyAudio), errors.Is(err, stt.ErrNotConfigured):
				return fail400(c, err)
			case errors.Is(err, stt.ErrAudioTooLarge):
				return sttAudioTooLarge(c)
			default:
				return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"success": false, "error": err.Error()})
			}
		}
		raw, err := protoMarshal.Marshal(&consolev1.SttTranscript{Text: text, Model: stt.Model})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
	})
}

func sttStatusToProto(status stt.Status) *consolev1.SttStatus {
	out := &consolev1.SttStatus{
		Configured:    proto.Bool(status.Configured),
		Model:         status.Model,
		MaxAudioBytes: uint32(status.MaxAudioBytes),
	}
	if status.Source != "" {
		out.Source = proto.String(status.Source)
	}
	if status.AccountID != "" {
		out.AccountId = proto.String(status.AccountID)
	}
	if !status.UpdatedAt.IsZero() {
		out.UpdatedAt = proto.String(status.UpdatedAt.UTC().Format(time.RFC3339))
	}
	return out
}

func respondSttStatus(c *fiber.Ctx, status stt.Status) error {
	raw, err := protoMarshal.Marshal(sttStatusToProto(status))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
	}
	return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
}

func respondSttConfig(c *fiber.Ctx, status stt.Status, removed bool) error {
	raw, err := protoMarshal.Marshal(&consolev1.SetSttConfigResponse{
		Status:  sttStatusToProto(status),
		Removed: removed,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
	}
	return c.JSON(fiber.Map{"success": true, "data": json.RawMessage(raw)})
}

func sttAudioTooLarge(c *fiber.Ctx) error {
	return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"success": false, "error": stt.ErrAudioTooLarge.Error()})
}

func sttInternalError(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": err.Error()})
}
