package data

import (
	"github.com/segmentio/ksuid"
)

type Data struct {
	Workspaces map[ksuid.KSUID]*Workspace
	Catalogs   map[ksuid.KSUID]*Catalog
}

type Metadata struct {
	Name string
}

type SystemConfiguration struct {
}

type System struct {
	Databases     []*Metadata
	Configuration SystemConfiguration
}

type RouterAgentConfig struct {
	Instructions string
}

type AgentReference struct {
	CatalogId ksuid.KSUID
	AgentId   ksuid.KSUID
}

type AgentRegistry struct {
	Agents []AgentReference
}

type WorkspaceConfig struct {
	RouterAgentConfig RouterAgentConfig
}

type Chat struct {
}

type WorkspaceState struct{}

type Workspace struct {
	ID   ksuid.KSUID
	Name string

	Config *WorkspaceConfig

	State *WorkspaceState // Need to abstract this to it's own type
	Chats map[string]*Chat
}

type Agent struct {
	ID           ksuid.KSUID
	Instructions string
}

type Catalog struct {
	ID   ksuid.KSUID
	Name string

	Agents []*Agent
}
