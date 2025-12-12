package connection

import (
	"log/slog"
	"net"
	"slate/internal/data"
	"slate/internal/parser"
	"slate/internal/scheduler"

	"github.com/segmentio/ksuid"
)

type Handler struct {
	context Context

	Connection net.Conn

	logger        *slog.Logger
	requestParser *parser.Parser
	store         *data.Data
	sched         *scheduler.Scheduler

	opts *Options
}

type Options struct {
	ClientIdleTimeout int
}

type Context struct {
	IPAddress string
	SessionID ksuid.KSUID
}
