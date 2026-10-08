// File browser & operations routes (/api/fs/*).
//
// Seventh domain on the shared protobuf schema. All JSON payloads are built
// from console.v1 generated types; the {success, data} envelope, SSE framing,
// ETag/304 behavior, and raw-bytes endpoints are unchanged. Trimmed shapes
// (see proto/console/v1/fs.proto): entry git status, Rust-only modified_at
// and is_binary, and window duration never crossed populated and are gone;
// grep counts narrow to uint32 so they stay JSON numbers.
package routes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/proto"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/fff"
	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func fsTreeEntryToProto(e types.FsTreeEntry) *consolev1.FsTreeEntry {
	out := &consolev1.FsTreeEntry{Name: e.Name, Path: e.Path, IsDir: e.IsDir}
	if e.Size != nil {
		out.Size = func() *uint64 { v := uint64(*e.Size); return &v }()
	}
	for _, c := range e.Children {
		out.Children = append(out.Children, fsTreeEntryToProto(c))
	}
	return out
}

func fsEntriesToProto(list []types.FsTreeEntry) []*consolev1.FsTreeEntry {
	out := make([]*consolev1.FsTreeEntry, 0, len(list))
	for _, e := range list {
		out = append(out, fsTreeEntryToProto(e))
	}
	return out
}

func grepResultToProto(r types.GrepResult) *consolev1.GrepResult {
	out := &consolev1.GrepResult{
		TotalMatched: uint32(r.TotalMatched), FilteredFiles: uint32(r.FilteredFiles),
		NextCursor: r.NextCursor, HasMore: r.HasMore,
	}
	if r.RegexError != "" {
		out.RegexError = &r.RegexError
	}
	for _, m := range r.Matches {
		match := &consolev1.GrepMatch{
			RelPath: m.RelPath, LineNumber: uint32(m.LineNumber), LineContent: m.LineContent,
		}
		for _, rng := range m.MatchRanges {
			match.MatchRanges = append(match.MatchRanges, &consolev1.GrepMatchRange{
				Start: uint32(rng.Start), End: uint32(rng.End),
			})
		}
		out.Matches = append(out.Matches, match)
	}
	return out
}

// entriesResponseCache holds encoded /api/fs/entries bodies tagged with the
// watcher version they were built at. The TTL is a backstop for missed events.
type entriesResponseCache struct {
	mu    sync.Mutex
	items map[string]entriesCacheItem
}

type entriesCacheItem struct {
	version uint64
	at      time.Time
	body    []byte
}

const (
	entriesCacheTTL = 30 * time.Second
	entriesCacheMax = 32
)

var entriesCache = &entriesResponseCache{items: make(map[string]entriesCacheItem)}

func (c *entriesResponseCache) get(key string, version uint64) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, ok := c.items[key]
	if !ok || item.version != version || time.Since(item.at) > entriesCacheTTL {
		return nil, false
	}
	return item.body, true
}

func (c *entriesResponseCache) put(key string, version uint64, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.items) >= entriesCacheMax {
		c.items = make(map[string]entriesCacheItem)
	}
	c.items[key] = entriesCacheItem{version: version, at: time.Now(), body: body}
}

func RegisterFsRoutes(app *fiber.App, fs *services.FsService, watch *services.FsWatchService) {
	h := app.Group("/api/fs")

	ok := func(c *fiber.Ctx, data any) error {
		return c.JSON(fiber.Map{"success": true, "data": data})
	}
	okProto := func(c *fiber.Ctx, msg proto.Message) error {
		raw, err := protoMarshal.Marshal(msg)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return ok(c, json.RawMessage(raw))
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
		msg := &consolev1.FsBrowseResult{CurrentPath: result.CurrentPath, Entries: fsEntriesToProto(result.Entries)}
		if result.ParentPath != nil {
			msg.ParentPath = result.ParentPath
		}
		return okProto(c, msg)
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
		out := make([]*consolev1.FileSearchResult, 0, len(items))
		for _, item := range items {
			out = append(out, &consolev1.FileSearchResult{
				RelativePath: item.RelativePath, AbsolutePath: item.AbsolutePath,
				IsDir: item.IsDir, Score: item.Score,
			})
		}
		data, err := marshalProtoList(out)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		return ok(c, data)
	})

	// GET /api/fs/grep — content search for the global search panel
	// (⌘⇧F): Aa case, ab whole-word, .* regex toggles.
	h.Get("/grep", func(c *fiber.Ctx) error {
		root := c.Query("root")
		if root == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Missing required query param: root"))
		}
		query := c.Query("q")
		if query == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Missing required query param: q"))
		}
		mode := fff.GrepModeRegex
		switch c.Query("mode") {
		case "plain":
			mode = fff.GrepModePlain
		case "fuzzy":
			mode = fff.GrepModeFuzzy
		}
		caseMode, err := fff.ParseCaseMode(c.Query("caseMode"))
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		maxMatches := clampInt(c.QueryInt("limit", 200), 1, 2000)
		result, err := fs.Grep(root, query, services.GrepOptions{
			Mode:         mode,
			Case:         caseMode,
			WholeWord:    c.Query("wholeWord") == "true",
			ContextLines: clampInt(c.QueryInt("contextLines", 0), 0, 20),
			MaxMatches:   maxMatches,
			Cursor:       uint32(c.QueryInt("cursor", 0)),
		})
		if err != nil {
			if errors.Is(err, services.ErrFffUnavailable) {
				return fail(c, fiber.StatusServiceUnavailable, err)
			}
			return fail(c, fiber.StatusBadRequest, err)
		}
		return okProto(c, grepResultToProto(result))
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
		withSizes := c.Query("withSizes") != "false" && c.Query("withSizes") != "0"
		hidden := c.Query("hidden") == "true"

		// Re-listing and re-encoding a deep tree is expensive and clients refetch
		// it on every fs event, so serve the encoded body until the watcher sees
		// a change. Hidden listings include ignored dirs the watcher skips, so
		// they are never cached.
		cacheable := !hidden
		key := fmt.Sprintf("%s|%d|%d|%t", dirPath, maxDepth, maxEntries, withSizes)
		var version uint64
		if cacheable {
			watch.Watch(dirPath)
			version = watch.Version()
			if body, hit := entriesCache.get(key, version); hit {
				c.Set("Content-Type", "application/json")
				return c.Send(body)
			}
		}

		entries, err := fs.ListAllEntries(dirPath, maxDepth, hidden,
			services.EntriesOptions{WithSizes: withSizes, MaxEntries: maxEntries})
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		data, err := marshalProtoList(fsEntriesToProto(entries))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "encode failed"})
		}
		body := append(append([]byte(`{"success":true,"data":`), data...), '}')
		if cacheable {
			// version was read before listing, so a change during the walk
			// leaves this entry already stale and it is rebuilt next time.
			entriesCache.put(key, version, body)
		}
		c.Set("Content-Type", "application/json")
		return c.Send(body)
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
		return okProto(c, &consolev1.FsDirectoryTree{Path: dirPath, TreeFormatted: tree})
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
		return okProto(c, &consolev1.FsFileContent{Path: filePath, Content: result.Content})
	})

	// POST /api/fs/file
	h.Post("/file", func(c *fiber.Ctx) error {
		var body consolev1.WriteFileRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &body); err != nil || body.Path == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Field 'path' is required."))
		}
		msg, err := fs.WriteFileContent(body.Path, body.Content)
		if err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return okProto(c, &consolev1.FileWriteResponse{Path: body.Path, Message: msg})
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
		return okProto(c, &consolev1.FileDeleteResponse{Path: filePath, Deleted: true})
	})

	// POST /api/fs/dir
	h.Post("/dir", func(c *fiber.Ctx) error {
		var body consolev1.CreateDirRequest
		if err := protoUnmarshal.Unmarshal(c.Body(), &body); err != nil || body.Path == "" {
			return fail(c, fiber.StatusBadRequest, fmt.Errorf("Field 'path' is required."))
		}
		if _, err := fs.CreateDirectory(body.Path); err != nil {
			return fail(c, fiber.StatusBadRequest, err)
		}
		return okProto(c, &consolev1.DirCreateResponse{Path: body.Path, Created: true})
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
		return okProto(c, &consolev1.DirDeleteResponse{Path: dirPath, Deleted: true})
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
					msg := &consolev1.FsChangeEvent{Type: evt.Type, ProjectPath: evt.ProjectPath}
					if evt.EventPath != "" {
						msg.EventPath = &evt.EventPath
					}
					raw, err := protoMarshal.Marshal(msg)
					if err != nil {
						return
					}
					if err := sse.Send("fsChange", string(raw)); err != nil {
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
	if ok := errorsAs(err, &blocked); ok {
		return blocked.Status
	}
	return fiber.StatusBadRequest
}
