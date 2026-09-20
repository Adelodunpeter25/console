// File browser & operations routes (/api/fs/*). Port of
// apps/server/api/src/routes/fs.ts.
package routes

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
)

func registerFsRoutes(app *fiber.App, fs *services.FsService, watch *services.FsWatchService) {
	h := app.Group("/api/fs")

	ok := func(c *fiber.Ctx, data any) error {
		return c.JSON(fiber.Map{"success": true, "data": data})
	}
	fail := func(c *fiber.Ctx, status int, err error) error {
		resp := fiber.Map{"success": false, "error": err.Error()}
		var blocked *services.PreviewBlocked
		if asErr(err, &blocked) {
			resp["code"] = blocked.Code
			for k, v := range blocked.Detail {
				resp[k] = v
			}
		}
		return c.Status(status).JSON(resp)
	}

	// GET /api/fs/browse
	h.Get("/browse", func(c *fiber.Ctx) error {
		result, err := fs.BrowseDirectory(c.Query("path"), c.Query("hidden") == "true")
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return ok(c, result)
	})

	// GET /api/fs/search
	h.Get("/search", func(c *fiber.Ctx) error {
		root := c.Query("root")
		if root == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Missing required query param: root"))
		}
		limit := clampInt(c.QueryInt("limit", 20), 1, 100)
		includeDirs := c.Query("includeDirs") != "false" && c.Query("includeDirs") != "0"
		items, err := fs.SearchFiles(root, c.Query("q", ""), limit, includeDirs)
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return ok(c, items)
	})

	// GET /api/fs/entries
	h.Get("/entries", func(c *fiber.Ctx) error {
		dirPath := c.Query("path")
		if dirPath == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Query parameter 'path' is required."))
		}
		maxDepth := c.QueryInt("depth", 6)
		if maxDepth < 1 || maxDepth > 25 {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Query parameter 'depth' must be an integer between 1 and 25."))
		}
		maxEntries := clampInt(c.QueryInt("maxEntries", 30000), 1, 100000)
		entries, err := fs.ListAllEntries(dirPath, maxDepth, c.Query("hidden") == "true",
			services.EntriesOptions{WithSizes: c.Query("withSizes") != "false" && c.Query("withSizes") != "0", MaxEntries: maxEntries})
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return ok(c, entries)
	})

	// GET /api/fs/tree
	h.Get("/tree", func(c *fiber.Ctx) error {
		dirPath := c.Query("path")
		if dirPath == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Query parameter 'path' is required."))
		}
		maxDepth := c.QueryInt("depth", 3)
		if maxDepth < 1 || maxDepth > 10 {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Query parameter 'depth' must be an integer between 1 and 10."))
		}
		tree, err := fs.GetDirectoryTree(dirPath, maxDepth, c.Query("hidden") == "true")
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return ok(c, fiber.Map{"path": dirPath, "treeFormatted": tree})
	})

	// GET /api/fs/file/raw — raw bytes for image/SVG preview with ETag/304.
	h.Get("/file/raw", func(c *fiber.Ctx) error {
		filePath := c.Query("path")
		if filePath == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Query parameter 'path' is required."))
		}
		meta, err := fs.GetImageMeta(filePath)
		if err != nil {
			return fail(c, previewStatus(err), err)
		}
		etag := services.BuildFileETag(meta.SizeBytes, meta.MtimeMs)
		if c.Get("If-None-Match") == etag {
			return c.Status(fiber.StatusNotModified).Send(nil)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		c.Set("Content-Type", meta.MimeType)
		c.Set("Content-Length", strconv.Itoa(len(data)))
		c.Set("Cache-Control", "private, max-age=30")
		c.Set("ETag", etag)
		return c.Send(data)
	})

	// GET /api/fs/file
	h.Get("/file", func(c *fiber.Ctx) error {
		filePath := c.Query("path")
		if filePath == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Query parameter 'path' is required."))
		}
		result, err := fs.ReadFileContentWithMeta(filePath, c.QueryInt("startLine", 0), c.QueryInt("endLine", 0))
		if err != nil {
			return fail(c, previewStatus(err), err)
		}
		etag := services.BuildFileETag(result.SizeBytes, result.MtimeMs)
		if c.Get("If-None-Match") == etag {
			return c.Status(fiber.StatusNotModified).Send(nil)
		}
		c.Set("ETag", etag)
		c.Set("Cache-Control", "private, max-age=5")
		return c.JSON(fiber.Map{"success": true, "data": fiber.Map{"path": filePath, "content": result.Content}})
	})

	// POST /api/fs/file
	h.Post("/file", func(c *fiber.Ctx) error {
		var body struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := c.BodyParser(&body); err != nil || body.Path == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Field 'path' is required."))
		}
		msg, err := fs.WriteFileContent(body.Path, body.Content)
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return ok(c, fiber.Map{"path": body.Path, "message": msg})
	})

	// DELETE /api/fs/file
	h.Delete("/file", func(c *fiber.Ctx) error {
		filePath := c.Query("path")
		if filePath == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Query parameter 'path' is required."))
		}
		if _, err := fs.DeleteFile(filePath); err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return ok(c, fiber.Map{"path": filePath, "deleted": true})
	})

	// POST /api/fs/dir
	h.Post("/dir", func(c *fiber.Ctx) error {
		var body struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&body); err != nil || body.Path == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Field 'path' is required."))
		}
		if _, err := fs.CreateDirectory(body.Path); err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return ok(c, fiber.Map{"path": body.Path, "created": true})
	})

	// DELETE /api/fs/dir
	h.Delete("/dir", func(c *fiber.Ctx) error {
		dirPath := c.Query("path")
		if dirPath == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Query parameter 'path' is required."))
		}
		if _, err := fs.DeleteDirectory(dirPath); err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return ok(c, fiber.Map{"path": dirPath, "deleted": true})
	})

	// GET /api/fs/watch — SSE stream of debounced fsChange events.
	h.Get("/watch", func(c *fiber.Ctx) error {
		projectPath := c.Query("path")
		if projectPath == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Query parameter 'path' is required."))
		}
		watch.Watch(projectPath)
		return streamSSE(c, func(sse *sseStream) {
			events := watch.Subscribe(projectPath)
			defer watch.Unsubscribe(events)
			// Event pump + heartbeat in the stream goroutine; Send's error
			// signals a disconnected client.
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case evt := <-events:
					if err := sse.Send("fsChange", mustJSON(evt)); err != nil {
						return
					}
				case <-ticker.C:
					if err := sse.Send("ping", ""); err != nil {
						return
					}
				}
			}
		})
	})
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func asErr(err error, target **services.PreviewBlocked) bool {
	var b *services.PreviewBlocked
	if ok := errorsAs(err, &b); ok {
		*target = b
		return true
	}
	return false
}

func previewStatus(err error) int {
	var blocked *services.PreviewBlocked
	if errorsAs(err, &blocked) {
		return blocked.Status
	}
	return fiber.StatusBadRequest
}
