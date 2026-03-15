package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"slate/cmd/server/configuration"
	"slate/internal/agent"
	"slate/internal/connection"
	"slate/internal/data"
	"slate/internal/events"
	"slate/internal/llm"
	"slate/internal/metrics"
	"slate/internal/scheduler"
	"slate/internal/tools"
	"slate/internal/tools/builtin"
	"slate/internal/trace"
)

func main() {
	c := configuration.New()
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

	sched := scheduler.NewScheduler(cfg.Workers)
	sched.Start()
	defer sched.Stop()

	store, err := data.New("default", cfg.DataDir)
	if err != nil {
		logger.Error("Creating Database", "error", err)
		return
	}
	defer closeDatabase(logger, store)

	met := metrics.New()

	evLogger, err := events.NewLogger(cfg.DataDir)
	if err != nil {
		logger.Error("Creating Event Logger", "error", err)
		return
	}

	tracer, err := trace.NewTracer(cfg.DataDir)
	if err != nil {
		logger.Error("Creating Tracer", "error", err)
		return
	}

	registry := tools.NewRegistry()
	registry.Register(builtin.NewHTTPFetchTool())
	registry.Register(builtin.NewShellTool())
	registry.Register(builtin.NewFileTool())

	extAgents := agent.NewExternalAgentRegistry()

	provider := llm.NewAnthropicProvider()
	runner := agent.NewRunner(provider, store, registry, agent.RunnerOptions{
		Logger:         logger,
		Metrics:        met,
		Events:         evLogger,
		ExternalAgents: extAgents,
		Tracer:         tracer,
	})
	runner.RegisterCallAgentTool(registry)

	// Close the listener when the context is cancelled so Accept unblocks.
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	sem := make(chan struct{}, cfg.MaxConnections)

	for {
		c, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return // clean shutdown
			}
			logger.Error("Accepting Connection", "error", err)
			continue
		}

		logger.Info("Incoming Connection")

		select {
		case sem <- struct{}{}:
			go func(c net.Conn) {
				conn := connection.New(c, sched, store, runner, met, extAgents, tracer, &connection.Options{
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

func closeDatabase(logger *slog.Logger, db *data.Data) {
	logger.Info("Closing Database")
	err := db.Close()
	if err != nil {
		logger.Error("Closing Database", "error", err)
	}
}
