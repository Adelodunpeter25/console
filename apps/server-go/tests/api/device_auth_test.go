package tests

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/auth"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/routes"
	"github.com/gofiber/fiber/v2"
)

func TestPairingGrantIsSingleUseAndProtectsRoutes(t *testing.T) {
	database, err := db.Open(db.OpenOptions{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	issuer := auth.NewTokenIssuerWithSecret([]byte("pairing-test-secret-32-bytes-long"))
	manager := auth.NewManager(database, issuer)
	grant, _, err := manager.CreateBootstrapGrant()
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	routes.RegisterPairingRoutes(app, manager)
	app.Use(routes.RequireDeviceToken(manager))
	app.Get("/private", func(c *fiber.Ctx) error { return c.SendString("ok") })

	unauthorized, _ := app.Test(httptest.NewRequest("GET", "/private", nil))
	if unauthorized.StatusCode != 401 {
		t.Fatalf("private route status = %d", unauthorized.StatusCode)
	}

	body := `{"grant":"` + grant + `","deviceName":"Desktop","platform":"desktop"}`
	pairReq := httptest.NewRequest("POST", "/api/auth/pair", newBody(body))
	pairReq.Header.Set("Content-Type", "application/json")
	response, err := app.Test(pairReq)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("pair status = %d", response.StatusCode)
	}
	var paired struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&paired); err != nil {
		t.Fatal(err)
	}
	if paired.Data.Token == "" {
		t.Fatal("pair response omitted token")
	}

	authorizedReq := httptest.NewRequest("GET", "/private", nil)
	authorizedReq.Header.Set("Authorization", "Bearer "+paired.Data.Token)
	authorized, _ := app.Test(authorizedReq)
	if authorized.StatusCode != 200 {
		t.Fatalf("authorized route status = %d", authorized.StatusCode)
	}

	reuseReq := httptest.NewRequest("POST", "/api/auth/pair", newBody(body))
	reuseReq.Header.Set("Content-Type", "application/json")
	reused, _ := app.Test(reuseReq)
	if reused.StatusCode != 401 {
		t.Fatalf("reused grant status = %d", reused.StatusCode)
	}
}

type requestBody string

func newBody(value string) *strings.Reader { return strings.NewReader(value) }
