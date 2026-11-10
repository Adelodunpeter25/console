package console

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
	projectssvc "github.com/Adelodunpeter25/console/apps/server-go/internal/services/projects"
)

func (r *Runtime) projectsList(_ context.Context, _ map[string]any) (*jskernel.Result, error) {
	if r.projects == nil {
		return nil, fmt.Errorf("project service is unavailable")
	}
	projects, err := r.projects.List()
	if err != nil {
		return nil, err
	}
	return marshalResult(projects)
}

func (r *Runtime) projectsAdd(_ context.Context, args map[string]any) (*jskernel.Result, error) {
	if r.projects == nil {
		return nil, fmt.Errorf("project service is unavailable")
	}
	dir, err := stringArg(args, "directory")
	if err != nil {
		return nil, err
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("directory does not exist: %s", dir)
	}
	name, _ := args["name"].(string)
	if name == "" {
		name = filepath.Base(filepath.Clean(dir))
	}
	project, err := r.projects.Create(projectssvc.CreateProjectOptions{Name: name, Dir: dir})
	if err != nil {
		return nil, err
	}
	return marshalResult(project)
}
