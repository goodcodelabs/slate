package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"slate/cmd/server/configuration"
	"slate/internal/connection"
)

func main() {
	c := configuration.NewConfiguration()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	run(c, logger)
}

func run(cfg *configuration.Configuration, logger *slog.Logger) {

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Host, cfg.Port))
	if err != nil {
		logger.Error("Starting Server", "error", err)
		return
	}
	defer closeListener(logger, listener)

	sem := make(chan struct{}, cfg.MaxConnections)
	for {
		c, err := listener.Accept()
		if err != nil {
			logger.Error("Accepting Connection", "error", err)
			continue
		}

		logger.Info("Incoming Connection")

		select {
		case sem <- struct{}{}:
			go func(c net.Conn) {
				conn := connection.New(c, &connection.Options{
					ClientIdleTimeout: cfg.ClientIdleTimeout,
				})

				defer func() { <-sem }()
				conn.HandleConnection(ctx)
			}(c)

		default:
			logger.Error("Too Many Connections")
			_, _ = c.Write([]byte("ERR too many connections\r\n"))
			_ = c.Close()
		}

	}
}

func closeListener(logger *slog.Logger, l net.Listener) {
	logger.Info("Closing Listener")
	err := l.Close()
	if err != nil {
		logger.Error("Closing Listener", "error", err)
	}
}
