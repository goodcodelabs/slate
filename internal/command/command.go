package command

import (
	"slate/internal/agent"
	"slate/internal/data"
	"slate/internal/metrics"
	"slate/internal/scheduler"

	"github.com/segmentio/ksuid"
)

// Command names
const (
	CmdAddWorkspace         = "add_workspace"
	CmdDelWorkspace         = "del_workspace"
	CmdAddCatalog           = "add_catalog"
	CmdDelCatalog           = "del_catalog"
	CmdListCatalogs         = "ls_catalogs"
	CmdHealth               = "health"
	CmdAddAgent             = "add_agent"
	CmdSetAgentInstructions = "set_agent_instructions"
	CmdSetAgentModel        = "set_agent_model"
	CmdRunAgent             = "run_agent"
	CmdNewThread            = "new_thread"
	CmdChat                 = "chat"
	CmdListThreads          = "ls_threads"
	CmdThreadHistory        = "thread_history"
	CmdAddTool              = "add_tool"
	CmdRemoveTool           = "remove_tool"
	CmdListTools            = "ls_tools"
	CmdSetWorkspaceCatalog  = "set_workspace_catalog"
	CmdSetWorkspaceRouter   = "set_workspace_router"
	CmdWorkspaceChat        = "workspace_chat"
	CmdCreatePipeline       = "create_pipeline"
	CmdAddPipelineStep      = "add_pipeline_step"
	CmdRunPipeline          = "run_pipeline"
	CmdJobStatus      = "job_status"
	CmdJobResult      = "job_result"
	CmdSystemMetrics  = "system_metrics"
	CmdSystemStats    = "system_stats"
	CmdListJobs       = "ls_jobs"
	CmdCancelJob      = "cancel_job"
)

type Command interface {
	Execute(commandContext Context, params []string) (*Response, error)
}
type ProtocolCommand struct {
	cmd Command
}

type Response struct {
	Message string
}

type Context struct {
	IPAddress string
	SessionID ksuid.KSUID
}

func (p *ProtocolCommand) Execute(commandContext Context, params []string) (*Response, error) {
	val, err := p.cmd.Execute(commandContext, params)
	if err != nil {
		return nil, err
	}
	return val, nil
}

func InitCommands(store *data.Data, runner *agent.Runner, sched *scheduler.Scheduler, met *metrics.Metrics) map[string]ProtocolCommand {
	return map[string]ProtocolCommand{
		CmdAddWorkspace:         {cmd: &AddWorkspaceCommand{store: store}},
		CmdDelWorkspace:         {cmd: &RemoveWorkspaceCommand{store: store}},
		CmdAddCatalog:           {cmd: &AddCatalogCommand{store: store}},
		CmdDelCatalog:           {cmd: &RemoveCatalogCommand{store: store}},
		CmdListCatalogs:         {cmd: &ListCatalogsCommand{store: store}},
		CmdHealth:               {cmd: &HealthCommand{}},
		CmdAddAgent:             {cmd: &AddAgentCommand{store: store}},
		CmdSetAgentInstructions: {cmd: &SetAgentInstructionsCommand{store: store}},
		CmdSetAgentModel:        {cmd: &SetAgentModelCommand{store: store}},
		CmdRunAgent:             {cmd: &RunAgentCommand{runner: runner}},
		CmdNewThread:            {cmd: &NewThreadCommand{store: store}},
		CmdChat:                 {cmd: &ChatCommand{runner: runner}},
		CmdListThreads:          {cmd: &ListThreadsCommand{store: store}},
		CmdThreadHistory:        {cmd: &ThreadHistoryCommand{store: store}},
		CmdAddTool:              {cmd: &AddToolCommand{store: store}},
		CmdRemoveTool:           {cmd: &RemoveToolCommand{store: store}},
		CmdListTools:            {cmd: &ListToolsCommand{store: store}},
		CmdSetWorkspaceCatalog:  {cmd: &SetWorkspaceCatalogCommand{store: store}},
		CmdSetWorkspaceRouter:   {cmd: &SetWorkspaceRouterCommand{store: store}},
		CmdWorkspaceChat:        {cmd: &WorkspaceChatCommand{runner: runner}},
		CmdCreatePipeline:       {cmd: &CreatePipelineCommand{store: store}},
		CmdAddPipelineStep:      {cmd: &AddPipelineStepCommand{store: store}},
		CmdRunPipeline:          {cmd: &RunPipelineCommand{store: store, runner: runner, sched: sched}},
		CmdJobStatus:     {cmd: &JobStatusCommand{store: store}},
		CmdJobResult:     {cmd: &JobResultCommand{store: store}},
		CmdSystemMetrics: {cmd: &SystemMetricsCommand{metrics: met}},
		CmdSystemStats:   {cmd: &SystemStatsCommand{store: store, sched: sched, metrics: met}},
		CmdListJobs:      {cmd: &ListJobsCommand{store: store}},
		CmdCancelJob:     {cmd: &CancelJobCommand{store: store}},
	}
}
