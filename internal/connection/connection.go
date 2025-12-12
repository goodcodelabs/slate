package connection

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"slate/internal/command"
	"slate/internal/data"
	"slate/internal/parser"
	"slate/internal/scheduler"
	"time"

	"github.com/segmentio/ksuid"
)

func New(connection net.Conn, sched *scheduler.Scheduler, store *data.Data, opts *Options) *Handler {
	connId := ksuid.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil)).With("conn_id", connId)
	requestParser := parser.New()

	return &Handler{
		Connection: connection,

		context: Context{
			IPAddress: getIPAddress(connection),
			SessionID: connId,
		},

		store: store,

		logger:        logger,
		requestParser: requestParser,
		sched:         sched,
		opts:          getConfiguration(opts),
	}
}

func (h *Handler) HandleConnection(ctx context.Context) {
	defer h.CloseConnection()

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

		commands := command.InitCommands(h.store)

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
