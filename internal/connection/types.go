package connection

import (
	"log/slog"
	"net"
	"slate/internal/agent"
	"slate/internal/data"
	"slate/internal/metrics"
	"slate/internal/parser"
	"slate/internal/scheduler"

	"github.com/segmentio/ksuid"
)

type Handler struct {
	context Context

	Connection net.Conn

	logger         *slog.Logger
	requestParser  *parser.Parser
	store          *data.Data
	runner         *agent.Runner
	sched          *scheduler.Scheduler
	metrics        *metrics.Metrics
	externalAgents *agent.ExternalAgentRegistry

	opts *Options
}

type Options struct {
	ClientIdleTimeout int
}

type Context struct {
	IPAddress string
	SessionID ksuid.KSUID
}
