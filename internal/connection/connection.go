package connection

import (
	"bufio"
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
		reader:     bufio.NewReader(connection),

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
		commands:      command.InitCommands(store, runner, sched, met),
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

	for {
		select {
		case <-ctx.Done():
			h.logger.Info("Server shutting down (context canceled)")
			return
		default:
		}

		_ = h.Connection.SetReadDeadline(time.Now().Add(time.Duration(h.opts.ClientIdleTimeout) * time.Millisecond))
		line, err := h.reader.ReadString('\n')
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

		line = strings.TrimRight(line, "\r\n")
		h.logger.Info("Request Received", "request", line)
		req, err := h.requestParser.ParseRequest(line)
		if err != nil {
			h.logger.Error("Parsing Request", "error", err)
			_ = h.respondError("invalid_syntax")
			continue
		}

		// Short circuit for quit
		if req.Command == "quit" {
			h.logger.Info("Client Disconnected")
			_ = h.respondOK("")
			return
		}

		// Short circuit for register_agent — switches connection to agent mode.
		if req.Command == "register_agent" {
			h.handleAgentRegistration(ctx, req.Params)
			return
		}

		h.sched.Schedule(&scheduler.Activity{
			Job: func() {
				cmd, ok := h.commands[req.Command]
				if !ok {
					h.logger.Error("Invalid Command", "command", req.Command)
					_ = h.respondError("invalid_command")
					return
				}

				resp, err := cmd.Execute(command.Context{
					IPAddress: h.context.IPAddress,
					SessionID: h.context.SessionID,
				}, req.Params)
				if err != nil {
					h.logger.Error("Executing Command", "error", err)
					_ = h.respondError(err.Error())
					return
				}

				if resp != nil {
					_ = h.respondOK(resp.Message)
				} else {
					_ = h.respondOK("")
				}
			},
		})
	}
}

// respondOK writes a successful JSON response.
// If data is empty or "ok", writes {"ok":true}; otherwise embeds data as raw JSON.
func (h *Handler) respondOK(data string) error {
	if data == "" || data == "ok" {
		return h.Respond(`{"ok":true}`)
	}
	return h.Respond(fmt.Sprintf(`{"ok":true,"data":%s}`, data))
}

// respondError writes a failure JSON response.
func (h *Handler) respondError(msg string) error {
	out, _ := json.Marshal(map[string]interface{}{"ok": false, "error": msg})
	return h.Respond(string(out))
}

func (h *Handler) Respond(msg string) error {
	_, err := h.Connection.Write([]byte(msg + "\n"))
	return err
}

func (h *Handler) CloseConnection() {
	err := h.Connection.Close()
	if err != nil {
		h.logger.Error("Closing Connection", "error", err)
	}
}

func getConfiguration(config *Options) *Options {
	timeout := config.ClientIdleTimeout
	if timeout == 0 {
		timeout = 6000
	}
	return &Options{ClientIdleTimeout: timeout}
}

func getIPAddress(conn net.Conn) string {
	if ra, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		return ra.IP.String()
	}
	return ""
}

// handleAgentRegistration processes register_agent with JSON params.
// It registers the current connection as an external agent, responds with the agent ID,
// then blocks until the server shuts down or the agent disconnects.
func (h *Handler) handleAgentRegistration(ctx context.Context, params json.RawMessage) {
	if h.externalAgents == nil {
		_ = h.respondError("external agent registration not enabled")
		return
	}

	var p struct {
		CatalogID    string `json:"catalog_id"`
		Name         string `json:"name"`
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(params, &p); err != nil || p.CatalogID == "" || p.Name == "" {
		_ = h.respondError(`usage: {"cmd":"register_agent","params":{"catalog_id":"...","name":"...","instructions":"..."}}`)
		return
	}

	catalogID, err := ksuid.Parse(p.CatalogID)
	if err != nil {
		_ = h.respondError(fmt.Sprintf("invalid catalog_id: %s", err))
		return
	}

	a, err := h.store.RegisterExternalAgent(catalogID, p.Name, p.Instructions)
	if err != nil {
		_ = h.respondError(err.Error())
		return
	}

	agentConn := h.externalAgents.Register(a.ID, h.Connection)
	defer h.externalAgents.Unregister(a.ID)

	h.logger.Info("External agent registered", "agent_id", a.ID, "name", a.Name)

	out, _ := json.Marshal(map[string]interface{}{
		"ok":   true,
		"data": map[string]string{"agent_id": a.ID.String()},
	})
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
