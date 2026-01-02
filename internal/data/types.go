package data

import (
	"github.com/segmentio/ksuid"
)

type Data struct {
	Workspaces map[ksuid.KSUID]*Workspace `msgpack:"workspaces"`
	Catalogs   map[ksuid.KSUID]*Catalog   `msgpack:"catalogs"`
	store      Store                      `msgpack:"-"` // Don't serialize the store
}

type Metadata struct {
	Name string `msgpack:"name"`
}

type SystemConfiguration struct {
}

type System struct {
	Databases     []*Metadata         `msgpack:"databases"`
	Configuration SystemConfiguration `msgpack:"configuration"`
}

type RouterAgentConfig struct {
	Instructions string `msgpack:"instructions"`
}

type AgentReference struct {
	CatalogId ksuid.KSUID `msgpack:"catalog_id"`
	AgentId   ksuid.KSUID `msgpack:"agent_id"`
}

type AgentRegistry struct {
	Agents []AgentReference `msgpack:"agents"`
}

type WorkspaceConfig struct {
	RouterAgentConfig RouterAgentConfig `msgpack:"router_agent_config"`
}

type Chat struct {
}

type WorkspaceState struct{}

type Workspace struct {
	ID   ksuid.KSUID `msgpack:"id"`
	Name string      `msgpack:"name"`

	Config *WorkspaceConfig `msgpack:"config"`

	State *WorkspaceState   `msgpack:"state"` // Need to abstract this to it's own type
	Chats map[string]*Chat  `msgpack:"chats"`
}

type Agent struct {
	ID           ksuid.KSUID `msgpack:"id"`
	Instructions string      `msgpack:"instructions"`
}

type Catalog struct {
	ID   ksuid.KSUID `msgpack:"id"`
	Name string      `msgpack:"name"`

	Agents []*Agent `msgpack:"agents"`
}
