package tools

import (
	"context"
	"errors"
	"path/filepath"

	workspacepkg "github.com/actonos/actonos/internal/workspace"
)

var (
	ErrPathEscape = errors.New("access denied: path escapes allowed workspace directory")
)

// CronSchedulerProvider defines interface for managing background cron schedules.
type CronSchedulerProvider interface {
	RegisterCron(id, agentID, cronExpr, prompt, targetChannel, targetAccountID, targetRecipient string) error
	RemoveCron(id string)
	ListCrons() []map[string]any
}

type NativeToolsConfig struct {
	DataDir       string
	AgentsDir     string
	UserWorkspace *workspacepkg.Store
}

// RegisterNativeTools adds native tools using the data directory inferred from
// the former workspace argument. New production wiring should use
// RegisterNativeToolsWithConfig so the user database and private agent roots
// are explicit.
func RegisterNativeTools(r *ToolRegistry, legacyWorkspaceDir string) {
	if legacyWorkspaceDir == "" {
		legacyWorkspaceDir = "./data/workspace"
	}
	dataDir := legacyWorkspaceDir
	if filepath.Base(legacyWorkspaceDir) == "workspace" {
		dataDir = filepath.Dir(legacyWorkspaceDir)
	}
	RegisterNativeToolsWithConfig(r, NativeToolsConfig{
		DataDir:   dataDir,
		AgentsDir: filepath.Join(dataDir, "agents"),
	})
}

func RegisterNativeToolsWithConfig(r *ToolRegistry, config NativeToolsConfig) {
	if config.DataDir == "" {
		config.DataDir = "./data"
	}
	if config.AgentsDir == "" {
		config.AgentsDir = filepath.Join(config.DataDir, "agents")
	}
	_ = r.Register(NewHTTPFetchTool())
	_ = r.Register(NewFileReadTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileWriteTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileEditTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileSearchTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileDeleteTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileMoveTool(config.DataDir, config.AgentsDir))
	_ = r.Register(NewFileCopyTool(config.DataDir, config.AgentsDir))
	execTool := NewExecTool(config.DataDir, config.AgentsDir)
	execTool.SetUserWorkspace(config.UserWorkspace)
	_ = r.Register(execTool)
	if config.UserWorkspace != nil {
		_ = r.Register(NewWorkspaceSearchTool(config.UserWorkspace))
		_ = r.Register(NewWorkspaceReadTool(config.UserWorkspace))
		_ = r.Register(NewWorkspaceWriteTool(config.UserWorkspace, config.DataDir, config.AgentsDir))
		_ = r.Register(NewWorkspaceDeleteTool(config.UserWorkspace))
	}
	_ = r.Register(NewWebSearchTool())
	notify := NewChannelNotifyTool(r.bus)
	notify.SetWorkspace(config.UserWorkspace, config.DataDir, config.AgentsDir)
	_ = r.Register(notify)
	_ = r.Register(NewSysInfoTool(config.DataDir))
	_ = r.Register(NewBrowserNavigateTool())
	_ = r.Register(NewBrowserScreenshotTool(config.AgentsDir))
	_ = r.Register(NewCronScheduleTool(nil))
	_ = r.Register(NewTaskEnqueueTool(nil))
}

// MissionBacklogProvider lets the agent enqueue unattended follow-up work.
type MissionBacklogProvider interface {
	EnqueueMission(ctx context.Context, title, description, assignedAgentID string) (id string, err error)
}

// AttachMissionBacklog connects native_task_enqueue to the autonomous task backlog.
func AttachMissionBacklog(r *ToolRegistry, backlog MissionBacklogProvider) {
	if r == nil {
		return
	}
	if t, err := r.Get("native_task_enqueue"); err == nil {
		if et, ok := t.(*TaskEnqueueTool); ok {
			et.SetBacklog(backlog)
			return
		}
	}
	_ = r.Register(NewTaskEnqueueTool(backlog))
}

// AttachCronScheduler connects the active CronScheduler to native_cron_schedule.
func AttachCronScheduler(r *ToolRegistry, scheduler CronSchedulerProvider) {
	if t, err := r.Get("native_cron_schedule"); err == nil {
		if ct, ok := t.(*CronScheduleTool); ok {
			ct.SetScheduler(scheduler)
		}
	} else {
		_ = r.Register(NewCronScheduleTool(scheduler))
	}
}
