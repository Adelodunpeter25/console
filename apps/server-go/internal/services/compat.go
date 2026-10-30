// Compatibility surface for code that moved out of this package into
// subpackages (git, fs, integrations, background, terminal, projects).
// Aliases and constructors here keep the old `services.X` spelling working
// for callers inside the server and the external tests package. New code
// should use the subpackage directly.
package services

import (
	"context"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/db"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/fff"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/background"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/fs"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/git"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/integrations"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/terminal"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// git
type (
	GitService      = git.GitService
	WorktreeService = git.WorktreeService
	WorktreeInfo    = git.WorktreeInfo
	WorktreeEntry   = git.WorktreeEntry
)

func NewGitService() *GitService           { return git.NewGitService() }
func NewWorktreeService() *WorktreeService { return git.NewWorktreeService() }
func RandomCodename() string               { return git.RandomCodename() }
func DefaultRoot() (string, error)         { return git.DefaultRoot() }

var (
	ErrWorktreeDirty      = git.ErrWorktreeDirty
	ErrUnbornHEAD         = git.ErrUnbornHEAD
	ErrNotGitRepo         = git.ErrNotGitRepo
	ErrUnknownBaseBranch  = git.ErrUnknownBaseBranch
	ErrGitHubNotConnected = projects.ErrGitHubNotConnected
)

// fs
type (
	FsService      = fs.FsService
	FsWatchService = fs.FsWatchService
	EntriesOptions = fs.EntriesOptions
	GrepOptions    = fs.GrepOptions
	PreviewBlocked = fs.PreviewBlocked
)

func NewFsService() *FsService { return fs.NewFsService() }
func NewFsWatchService() (*FsWatchService, error) {
	return fs.NewFsWatchService()
}
func SetFffManager(m *fff.Manager) { fs.SetFffManager(m) }
func BuildFileETag(sizeBytes int64, mtimeMs int64) string {
	return fs.BuildFileETag(sizeBytes, mtimeMs)
}

var ErrFffUnavailable = fs.ErrFffUnavailable

// integrations
type IntegrationInfo = integrations.IntegrationInfo

var (
	BrewBin               = integrations.BrewBin
	GhInstallOS           = integrations.GhInstallOS
	GhReleaseDownloadBase = integrations.GhReleaseDownloadBase
	GhReleaseAPIBase      = integrations.GhReleaseAPIBase
	GhReleaseAsset        = integrations.GhReleaseAsset

	ErrGhPlatformUnsupported = integrations.ErrGhPlatformUnsupported
)

func ListIntegrations() []IntegrationInfo { return integrations.ListIntegrations() }
func InstallGhCLI(ctx context.Context) (IntegrationInfo, error) {
	return integrations.InstallGhCLI(ctx)
}
func EnsureUserBinDir(env []string) []string { return integrations.EnsureUserBinDir(env) }
func CheckGhCLI() IntegrationInfo            { return integrations.CheckGhCLI() }

// background
type (
	BackgroundWork          = background.Work
	BackgroundWorkEvent     = background.Event
	BackgroundWorkKind      = background.Kind
	BackgroundWorkStatus    = background.Status
	BackgroundWorkEventType = background.EventType
	BackgroundWorkManager   = background.Manager
	BackgroundWorkOptions   = background.Options
)

var (
	BackgroundWorkShell          = background.KindShell
	BackgroundWorkSubagent       = background.KindSubagent
	BackgroundWorkRunning        = background.StatusRunning
	BackgroundWorkCompleted      = background.StatusCompleted
	BackgroundWorkFailed         = background.StatusFailed
	BackgroundWorkCancelled      = background.StatusCancelled
	BackgroundWorkTimedOut       = background.StatusTimedOut
	BackgroundWorkStartedEvent   = background.EventStarted
	BackgroundWorkProgressEvent  = background.EventProgress
	BackgroundWorkCompletedEvent = background.EventCompleted
	BackgroundWorkFailedEvent    = background.EventFailed
	BackgroundWorkCancelledEvent = background.EventCancelled
)

func NewBackgroundWorkManager() *BackgroundWorkManager { return background.NewManager() }

// terminal
type (
	PtyManager          = terminal.PtyManager
	PortRegistry        = terminal.PortRegistry
	PortOwner           = terminal.PortOwner
	BashJobManager      = terminal.BashJobManager
	BashJobChunk        = terminal.BashJobChunk
	BashJobSnapshot     = terminal.BashJobSnapshot
	BashJobOutput       = terminal.BashJobOutput
	BashJobStatus       = terminal.BashJobStatus
	StartBashJobOptions = terminal.StartBashJobOptions
	SpawnCaptureOptions = terminal.SpawnCaptureOptions
	SpawnCaptureResult  = terminal.SpawnCaptureResult
)

var (
	BashJobDefaultTimeoutMs = terminal.BashJobDefaultTimeoutMs
	BashJobMaxTimeoutMs     = terminal.BashJobMaxTimeoutMs
	BashJobRunning          = terminal.BashJobRunning
	BashJobExited           = terminal.BashJobExited
	BashJobFailed           = terminal.BashJobFailed
	BashJobKilled           = terminal.BashJobKilled
	ErrTargetNotEmpty       = projects.ErrTargetNotEmpty
	ErrInvalidRepoURL       = projects.ErrInvalidRepoURL
)

func NewPtyManager(ports *PortRegistry, projectByDir func(cwd string) string) *PtyManager {
	return terminal.NewPtyManager(ports, projectByDir)
}
func NewPortRegistry() *PortRegistry { return terminal.NewPortRegistry() }
func NewBashJobManager(ports *PortRegistry) *BashJobManager {
	return terminal.NewBashJobManager(ports)
}
func NewUTF8Decoder() *terminal.UTF8Decoder { return terminal.NewUTF8Decoder() }
func SpawnCapture(
	ctx context.Context, argv []string, opts SpawnCaptureOptions,
) SpawnCaptureResult {
	return terminal.SpawnCapture(ctx, argv, opts)
}

// projects
type (
	ProjectService          = projects.ProjectService
	CreateProjectOptions    = projects.CreateProjectOptions
	ProjectScriptsService   = projects.ProjectScriptsService
	ProjectBootstrapService = projects.ProjectBootstrapService
)

func NewProjectService(manager *db.DB) *ProjectService { return projects.NewProjectService(manager) }
func NewProjectScriptsService(proj *ProjectService, ports *PortRegistry) *ProjectScriptsService {
	return projects.NewProjectScriptsService(proj, ports)
}
func NewProjectBootstrapService() *ProjectBootstrapService {
	return projects.NewProjectBootstrapService()
}
func BuildScriptEnv() []string { return projects.BuildScriptEnv() }
func ParseProjectScriptsForTest(text string) ([]types.ProjectScript, error) {
	return projects.ParseProjectScriptsForTest(text)
}
