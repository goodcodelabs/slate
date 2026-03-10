package connection

import (
	"bufio"
	"log/slog"
	"net"
	"slate/internal/agent"
	"slate/internal/command"
	"slate/internal/data"
	"slate/internal/metrics"
	"slate/internal/parser"
	"slate/internal/scheduler"

	"github.com/segmentio/ksuid"
)

type Handler struct {
	context Context

	Connection net.Conn
	reader     *bufio.Reader

	logger         *slog.Logger
	requestParser  *parser.Parser
	commands       map[string]command.ProtocolCommand
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
