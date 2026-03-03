package data

import (
	"context"
	"errors"
	"time"

	"github.com/segmentio/ksuid"
	"slate/internal/llm"
)

func New(name, dataDir string) (*Data, error) {
	// Initialize file store
	store, err := NewFileStore(dataDir)
	if err != nil {
		return nil, err
	}

	// Load existing data from disk
	db, err := store.Load()
	if err != nil {
		return nil, err
	}

	// Attach store to data
	db.store = store

	// Ensure maps are always initialised (Load initialises them, but be safe)
	if db.Threads == nil {
		db.Threads = make(map[ksuid.KSUID]*Thread)
	}
	if db.Pipelines == nil {
		db.Pipelines = make(map[ksuid.KSUID]*Pipeline)
	}
	// Jobs are always fresh — they are not persisted across restarts.
	db.Jobs = make(map[ksuid.KSUID]*Job)

	return db, nil
}

func (db *Data) Close() error {
	if db.store != nil {
		return db.store.Close()
	}
	return nil
}

func (db *Data) AddWorkspace(name string) error {

	if db.workspaceExists(name) {
		return errors.New("workspace already exists")
	}

	w, err := newWorkspace(name, &WorkspaceConfig{})
	if err != nil {
		return err
	}

	db.Workspaces[w.ID] = w

	// Persist to disk
	if db.store != nil {
		if err := db.store.SaveWorkspace(w); err != nil {
			// Rollback in-memory change on persistence failure
			delete(db.Workspaces, w.ID)
			return err
		}
	}

	return nil
}

func (db *Data) workspaceExists(name string) bool {
	// Ensure there are no name collisions
	for _, w := range db.Workspaces {
		if w.Name == name {
			return true
		}
	}

	return false
}

func (db *Data) RemoveWorkspace(name string) error {

	var targetID ksuid.KSUID
	var found bool

	for _, w := range db.Workspaces {
		if w.Name == name {
			targetID = w.ID
			found = true
			break
		}
	}

	if !found {
		return errors.New("workspace not found")
	}

	// Remove from memory
	delete(db.Workspaces, targetID)

	// Persist to disk
	if db.store != nil {
		if err := db.store.DeleteWorkspace(targetID); err != nil {
			return err
		}
	}

	return nil
}

func (db *Data) AddCatalog(name string) error {
	if db.catalogExists(name) {
		return errors.New("catalog already exists")
	}

	c, err := newCatalog(name)
	if err != nil {
		return err
	}

	db.Catalogs[c.ID] = c

	// Persist to disk
	if db.store != nil {
		if err := db.store.SaveCatalog(c); err != nil {
			// Rollback in-memory change on persistence failure
			delete(db.Catalogs, c.ID)
			return err
		}
	}

	return nil
}

func (db *Data) catalogExists(name string) bool {
	for _, c := range db.Catalogs {
		if c.Name == name {
			return true
		}
	}
	return false
}

func (db *Data) RemoveCatalog(name string) error {

	var targetID ksuid.KSUID
	var found bool

	for _, c := range db.Catalogs {
		if c.Name == name {
			targetID = c.ID
			found = true
			break
		}
	}

	if !found {
		return errors.New("catalog not found")
	}

	// Remove from memory
	delete(db.Catalogs, targetID)

	// Persist to disk
	if db.store != nil {
		if err := db.store.DeleteCatalog(targetID); err != nil {
			return err
		}
	}

	return nil
}

func (db *Data) ListCatalogs() ([]Catalog, error) {
	cs := make([]Catalog, 0, len(db.Catalogs))
	for _, c := range db.Catalogs {
		cs = append(cs, *c)
	}
	return cs, nil
}

func (db *Data) AddAgent(catalogID ksuid.KSUID, name string) (*Agent, error) {
	catalog, ok := db.Catalogs[catalogID]
	if !ok {
		return nil, errors.New("catalog not found")
	}

	a := &Agent{
		ID:        ksuid.New(),
		Name:      name,
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
	}

	catalog.AddAgent(a)

	if db.store != nil {
		if err := db.store.SaveCatalog(catalog); err != nil {
			catalog.Agents = catalog.Agents[:len(catalog.Agents)-1]
			return nil, err
		}
	}

	return a, nil
}

func (db *Data) RegisterExternalAgent(catalogID ksuid.KSUID, name, instructions string) (*Agent, error) {
	catalog, ok := db.Catalogs[catalogID]
	if !ok {
		return nil, errors.New("catalog not found")
	}

	a := &Agent{
		ID:           ksuid.New(),
		Name:         name,
		Instructions: instructions,
		External:     true,
	}

	catalog.AddAgent(a)

	if db.store != nil {
		if err := db.store.SaveCatalog(catalog); err != nil {
			catalog.Agents = catalog.Agents[:len(catalog.Agents)-1]
			return nil, err
		}
	}

	return a, nil
}

func (db *Data) FindAgent(agentID ksuid.KSUID) (*Agent, *Catalog, error) {
	for _, catalog := range db.Catalogs {
		for _, agent := range catalog.Agents {
			if agent.ID == agentID {
				return agent, catalog, nil
			}
		}
	}
	return nil, nil, errors.New("agent not found")
}

func (db *Data) SetAgentInstructions(agentID ksuid.KSUID, instructions string) error {
	agent, catalog, err := db.FindAgent(agentID)
	if err != nil {
		return err
	}
	agent.Instructions = instructions
	if db.store != nil {
		return db.store.SaveCatalog(catalog)
	}
	return nil
}

func (db *Data) SetAgentModel(agentID ksuid.KSUID, model string) error {
	agent, catalog, err := db.FindAgent(agentID)
	if err != nil {
		return err
	}
	agent.Model = model
	if db.store != nil {
		return db.store.SaveCatalog(catalog)
	}
	return nil
}

func (db *Data) AddAgentTool(agentID ksuid.KSUID, toolName string) error {
	agent, catalog, err := db.FindAgent(agentID)
	if err != nil {
		return err
	}
	for _, t := range agent.Tools {
		if t == toolName {
			return errors.New("tool already attached")
		}
	}
	agent.Tools = append(agent.Tools, toolName)
	if db.store != nil {
		return db.store.SaveCatalog(catalog)
	}
	return nil
}

func (db *Data) RemoveAgentTool(agentID ksuid.KSUID, toolName string) error {
	agent, catalog, err := db.FindAgent(agentID)
	if err != nil {
		return err
	}
	idx := -1
	for i, t := range agent.Tools {
		if t == toolName {
			idx = i
			break
		}
	}
	if idx == -1 {
		return errors.New("tool not found on agent")
	}
	agent.Tools = append(agent.Tools[:idx], agent.Tools[idx+1:]...)
	if db.store != nil {
		return db.store.SaveCatalog(catalog)
	}
	return nil
}

func (db *Data) NewThread(workspaceID, agentID ksuid.KSUID, name string) (*Thread, error) {
	if _, ok := db.Workspaces[workspaceID]; !ok {
		return nil, errors.New("workspace not found")
	}
	if _, _, err := db.FindAgent(agentID); err != nil {
		return nil, errors.New("agent not found")
	}

	now := time.Now().UTC()
	t := &Thread{
		ID:          ksuid.New(),
		WorkspaceID: workspaceID,
		AgentID:     agentID,
		Name:        name,
		State:       ThreadActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	db.Threads[t.ID] = t

	if db.store != nil {
		if err := db.store.SaveThread(t); err != nil {
			delete(db.Threads, t.ID)
			return nil, err
		}
	}

	return t, nil
}

func (db *Data) GetThread(id ksuid.KSUID) (*Thread, error) {
	t, ok := db.Threads[id]
	if !ok {
		return nil, errors.New("thread not found")
	}
	return t, nil
}

func (db *Data) DeleteThread(id ksuid.KSUID) error {
	if _, ok := db.Threads[id]; !ok {
		return errors.New("thread not found")
	}

	delete(db.Threads, id)

	if db.store != nil {
		return db.store.DeleteThread(id)
	}
	return nil
}

func (db *Data) AppendMessage(threadID ksuid.KSUID, msg llm.Message) error {
	thread, ok := db.Threads[threadID]
	if !ok {
		return errors.New("thread not found")
	}

	thread.Messages = append(thread.Messages, msg)

	if db.store != nil {
		return db.store.AppendMessage(threadID, msg)
	}
	return nil
}

func (db *Data) ListThreads(workspaceID ksuid.KSUID) ([]*Thread, error) {
	if _, ok := db.Workspaces[workspaceID]; !ok {
		return nil, errors.New("workspace not found")
	}
	var ts []*Thread
	for _, t := range db.Threads {
		if t.WorkspaceID == workspaceID {
			ts = append(ts, t)
		}
	}
	return ts, nil
}

func (db *Data) GetWorkspace(id ksuid.KSUID) (*Workspace, error) {
	w, ok := db.Workspaces[id]
	if !ok {
		return nil, errors.New("workspace not found")
	}
	return w, nil
}

func (db *Data) GetCatalog(id ksuid.KSUID) (*Catalog, error) {
	c, ok := db.Catalogs[id]
	if !ok {
		return nil, errors.New("catalog not found")
	}
	return c, nil
}

func (db *Data) SetWorkspaceCatalog(workspaceID, catalogID ksuid.KSUID) error {
	workspace, ok := db.Workspaces[workspaceID]
	if !ok {
		return errors.New("workspace not found")
	}
	if _, ok := db.Catalogs[catalogID]; !ok {
		return errors.New("catalog not found")
	}
	workspace.CatalogID = catalogID
	if db.store != nil {
		return db.store.SaveWorkspace(workspace)
	}
	return nil
}

func (db *Data) SetWorkspaceRouter(workspaceID, agentID ksuid.KSUID) error {
	workspace, ok := db.Workspaces[workspaceID]
	if !ok {
		return errors.New("workspace not found")
	}
	if _, _, err := db.FindAgent(agentID); err != nil {
		return errors.New("agent not found")
	}
	if workspace.Config == nil {
		workspace.Config = &WorkspaceConfig{}
	}
	workspace.Config.RouterAgentID = agentID
	if db.store != nil {
		return db.store.SaveWorkspace(workspace)
	}
	return nil
}

func (db *Data) NewPipeline(workspaceID ksuid.KSUID, name string) (*Pipeline, error) {
	if _, ok := db.Workspaces[workspaceID]; !ok {
		return nil, errors.New("workspace not found")
	}
	p := &Pipeline{
		ID:          ksuid.New(),
		WorkspaceID: workspaceID,
		Name:        name,
	}
	db.Pipelines[p.ID] = p
	if db.store != nil {
		if err := db.store.SavePipeline(p); err != nil {
			delete(db.Pipelines, p.ID)
			return nil, err
		}
	}
	return p, nil
}

func (db *Data) GetPipeline(id ksuid.KSUID) (*Pipeline, error) {
	p, ok := db.Pipelines[id]
	if !ok {
		return nil, errors.New("pipeline not found")
	}
	return p, nil
}

func (db *Data) RemovePipeline(id ksuid.KSUID) error {
	if _, ok := db.Pipelines[id]; !ok {
		return errors.New("pipeline not found")
	}
	delete(db.Pipelines, id)
	if db.store != nil {
		return db.store.DeletePipeline(id)
	}
	return nil
}

func (db *Data) AddPipelineStep(pipelineID, agentID ksuid.KSUID, mode StepMode) error {
	pipeline, ok := db.Pipelines[pipelineID]
	if !ok {
		return errors.New("pipeline not found")
	}
	if _, _, err := db.FindAgent(agentID); err != nil {
		return errors.New("agent not found")
	}
	pipeline.Steps = append(pipeline.Steps, PipelineStep{
		AgentID: agentID,
		Mode:    mode,
	})
	if db.store != nil {
		return db.store.SavePipeline(pipeline)
	}
	return nil
}

func (db *Data) ListPipelines(workspaceID ksuid.KSUID) ([]*Pipeline, error) {
	if _, ok := db.Workspaces[workspaceID]; !ok {
		return nil, errors.New("workspace not found")
	}
	var ps []*Pipeline
	for _, p := range db.Pipelines {
		if p.WorkspaceID == workspaceID {
			ps = append(ps, p)
		}
	}
	return ps, nil
}

func (db *Data) CreateJob(jobType string, workspaceID, pipelineID ksuid.KSUID, input string) (*Job, error) {
	job := &Job{
		ID:          ksuid.New(),
		Type:        jobType,
		WorkspaceID: workspaceID,
		PipelineID:  pipelineID,
		Input:       input,
		Status:      JobPending,
		CreatedAt:   time.Now().UTC(),
	}
	db.Jobs[job.ID] = job
	return job, nil
}

func (db *Data) SetJobCancel(id ksuid.KSUID, cancel context.CancelFunc) error {
	job, ok := db.Jobs[id]
	if !ok {
		return errors.New("job not found")
	}
	job.CancelFunc = cancel
	return nil
}

func (db *Data) CancelJob(id ksuid.KSUID) error {
	job, ok := db.Jobs[id]
	if !ok {
		return errors.New("job not found")
	}
	if job.CancelFunc != nil {
		job.CancelFunc()
	}
	return nil
}

func (db *Data) ListJobs(workspaceID ksuid.KSUID) ([]*Job, error) {
	var jobs []*Job
	for _, j := range db.Jobs {
		if workspaceID == (ksuid.KSUID{}) || j.WorkspaceID == workspaceID {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (db *Data) GetJob(id ksuid.KSUID) (*Job, error) {
	job, ok := db.Jobs[id]
	if !ok {
		return nil, errors.New("job not found")
	}
	return job, nil
}

func (db *Data) UpdateJob(id ksuid.KSUID, status JobStatus, result, errMsg string) error {
	job, ok := db.Jobs[id]
	if !ok {
		return errors.New("job not found")
	}
	job.Status = status
	now := time.Now().UTC()
	switch status {
	case JobRunning:
		job.StartedAt = now
	case JobCompleted, JobFailed:
		job.CompletedAt = now
		job.Result = result
		job.Error = errMsg
	}
	return nil
}
