package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"

	"github.com/ccvass/swarmex/swarmex-gatekeeper"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		logger.Error("failed to create Docker client", "error", err)
		os.Exit(1)
	}
	defer cli.Close()

	gk := gatekeeper.New(cli, logger)

	// Health endpoint
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "ok")
		})
		logger.Info("health endpoint", "addr", ":8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			logger.Error("health server error", "error", err)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Info("swarmex-gatekeeper starting")

	// Listen to Docker events directly (inline event loop, same pattern as event-controller)
	msgCh, errCh := cli.Events(ctx, events.ListOptions{})
	for {
		select {
		case event := <-msgCh:
			gk.HandleEvent(ctx, event)
		case err := <-errCh:
			if ctx.Err() != nil {
				logger.Info("shutdown complete")
				return
			}
			logger.Error("event stream error", "error", err)
			return
		case <-ctx.Done():
			logger.Info("shutdown complete")
			return
		}
	}
}
