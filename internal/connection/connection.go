package connection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"slate/internal/agent"
	"slate/internal/command"
	"slate/internal/data"
	"slate/internal/metrics"
	"slate/internal/parser"
	"slate/internal/scheduler"
	"strings"
	"time"

	"github.com/segmentio/ksuid"
)

func New(connection net.Conn, sched *scheduler.Scheduler, store *data.Data, runner *agent.Runner, met *metrics.Metrics, extAgents *agent.ExternalAgentRegistry, opts *Options) *Handler {
	connId := ksuid.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil)).With("conn_id", connId)
	requestParser := parser.New()

	return &Handler{
		Connection: connection,

		context: Context{
			IPAddress: getIPAddress(connection),
			SessionID: connId,
		},

		store:          store,
		runner:         runner,
		metrics:        met,
		externalAgents: extAgents,

		logger:        logger,
		requestParser: requestParser,
		sched:         sched,
		opts:          getConfiguration(opts),
	}
}

func (h *Handler) HandleConnection(ctx context.Context) {
	defer h.CloseConnection()

	if h.metrics != nil {
		h.metrics.IncrConnections()
		defer h.metrics.DecrConnections()
	}

	h.logger.Info("Connection Started", "ip", h.context.IPAddress, "conn_id", h.context.SessionID)

	buffer := make([]byte, 16384)

	for {

		select {
		case <-ctx.Done():
			h.logger.Info("Server shutting down (context canceled)")
			return
		default:
		}

		_ = h.Connection.SetReadDeadline(time.Now().Add(time.Duration(h.opts.ClientIdleTimeout) * time.Millisecond))
		n, err := h.Connection.Read(buffer)
		if err != nil {
			switch {
			case errors.Is(err, io.EOF):
				h.logger.Info("Client closed connection (EOF)")
			case errors.Is(err, net.ErrClosed):
				h.logger.Info("Connection closed")
			default:
				var ne net.Error
				if ok := errors.As(err, &ne); ok && ne.Timeout() {
					h.logger.Warn("Client timeout", "error", err)
				} else {
					h.logger.Error("Reading Request", "error", err)
				}
			}
			return
		}

		h.logger.Info("Request Received", "request", string(buffer[:n]))
		req, err := h.requestParser.ParseRequest(string(buffer[:n]))
		if err != nil {
			h.logger.Error("Parsing Request", "error", err)
			_ = h.Respond("error|invalid_syntax")
			continue
		}

		// Short circuit for quit
		if req.Command == "quit" {
			h.logger.Info("Client Disconnected")
			_ = h.Respond("ok")
			return
		}

		// Short circuit for register_agent — switches connection to agent mode.
		if req.Command == "register_agent" {
			h.handleAgentRegistration(ctx, req.Params)
			return
		}

		commands := command.InitCommands(h.store, h.runner, h.sched, h.metrics)

		h.sched.Schedule(&scheduler.Activity{
			Job: func() {

				// check if req.Command is in commands
				_, ok := commands[req.Command]
				if !ok {
					h.logger.Error("Invalid Command", "command", req.Command)
					_ = h.Respond("error|invalid_command")
					return
				}

				cmd := commands[req.Command]
				resp, err := cmd.Execute(command.Context{
					IPAddress: h.context.IPAddress,
					SessionID: h.context.SessionID,
				}, req.Params)
				if err != nil {
					h.logger.Error("Executing Command", "error", err)
					_ = h.Respond(fmt.Sprintf("error|%s", err.Error()))
					return
				}

				if resp != nil {
					_ = h.Respond(resp.Message)
				}
			},
		})
	}

}

func (h *Handler) Respond(msg string) error {
	_, err := h.Connection.Write([]byte(msg + "\n"))
	return err
}

func (h *Handler) CloseConnection() {
	err := h.Respond("Closing Connection")
	if err != nil {
		h.logger.Error("Notifying Client", "error", err)
	}

	err = h.Connection.Close()
	if err != nil {
		h.logger.Error("Closing Connection", "error", err)
	}
}

func getConfiguration(config *Options) *Options {
	return &Options{
		ClientIdleTimeout: config.ClientIdleTimeout | 6000,
	}
}

func getIPAddress(conn net.Conn) string {
	if ra, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		return ra.IP.String()
	}
	return ""
}

// handleAgentRegistration processes register_agent <catalog_id> <name> <instructions...>.
// It registers the current connection as an external agent, responds with the agent ID,
// then blocks until the server shuts down or the agent disconnects.
func (h *Handler) handleAgentRegistration(ctx context.Context, params []string) {
	if h.externalAgents == nil {
		_ = h.Respond("error|external agent registration not enabled")
		return
	}
	if len(params) < 3 {
		_ = h.Respond("error|usage: register_agent <catalog_id> <name> <instructions...>")
		return
	}

	catalogID, err := ksuid.Parse(params[0])
	if err != nil {
		_ = h.Respond(fmt.Sprintf("error|invalid catalog_id: %s", err))
		return
	}
	name := params[1]
	instructions := strings.Join(params[2:], " ")

	a, err := h.store.RegisterExternalAgent(catalogID, name, instructions)
	if err != nil {
		_ = h.Respond(fmt.Sprintf("error|%s", err))
		return
	}

	agentConn := h.externalAgents.Register(a.ID, h.Connection)
	defer h.externalAgents.Unregister(a.ID)

	h.logger.Info("External agent registered", "agent_id", a.ID, "name", a.Name)

	out, _ := json.Marshal(map[string]string{"agent_id": a.ID.String()})
	if _, err := h.Connection.Write(append(out, '\n')); err != nil {
		return
	}

	// Hold the connection open until the server shuts down or the agent drops.
	select {
	case <-ctx.Done():
	case <-agentConn.Done():
	}

	h.logger.Info("External agent disconnected", "agent_id", a.ID)
}
