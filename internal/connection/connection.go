package connection

import (
	"context"
	"errors"
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

func New(connection net.Conn, opts *Options) *Handler {
	connId := ksuid.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil)).With("conn_id", connId)
	requestParser := parser.New()
	store := data.New()
	sched := scheduler.NewScheduler()

	return &Handler{
		Connection: connection,

		ID:        connId,
		IPAddress: getIPAddress(connection),

		logger:        logger,
		requestParser: requestParser,
		store:         store,
		sched:         sched,
		opts:          getConfiguration(opts),
	}
}

func (h *Handler) HandleConnection(ctx context.Context) {
	defer h.CloseConnection()

	h.logger.Info("Connection Started", "ip", h.IPAddress, "conn_id", h.ID)

	buffer := make([]byte, 1024)

	h.sched.Start()
	defer h.sched.Stop()

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
			_ = h.Respond("invalid_syntax")
			continue
		}

		setCommand := command.InitCommand(command.InitSetCommand(h.store))

		h.sched.Schedule(&scheduler.Activity{
			Job: func() {
				switch req.Command {
				case "quit":
					h.logger.Info("Client Disconnected")
					_ = h.Respond("ok")
					return
				case "set":
					if len(req.Params) != 2 {
						h.logger.Error("Invalid Set Command")
						_ = h.Respond("error|Invalid Parameters")
						return
					}
					err = setCommand.Cmd.Execute(req.Params)
					h.logger.Info("Set Command", "key", req.Params[0], "value", req.Params[1])
					_ = h.Respond("ok")
				case "get":
					if len(req.Params) != 1 {
						h.logger.Error("Invalid Get Command")
						_ = h.Respond("error|Invalid Parameters")
						return
					}
					val, err := h.store.Get(req.Params[0])
					if err != nil {
						h.logger.Error("Getting Key:", err)
						_ = h.Respond("error|Key Not Found")
						return
					}
					h.logger.Info("Get Command", "key", req.Params[0], "value", val)
					_ = h.Respond(val)
				case "del":
					if len(req.Params) != 1 {
						h.logger.Error("Invalid Del Command")
						_ = h.Respond("error|Invalid Parameters")
						return
					}
					err = h.store.Del(req.Params[0])
					if err != nil {
						h.logger.Error("Deleting Key:", err)
						_ = h.Respond("error|Key Not Found")
						return
					}
					h.logger.Info("Del Command", "key", req.Params[0])
					_ = h.Respond("ok")
				default:
					h.logger.Error("Invalid Command")
					_ = h.Respond("invalid_syntax")
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
