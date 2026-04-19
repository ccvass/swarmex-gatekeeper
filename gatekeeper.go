package gatekeeper

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
)

const (
	labelEnabled   = "swarmex.gatekeeper.enabled"
	labelPath      = "swarmex.gatekeeper.path"
	labelInterval  = "swarmex.gatekeeper.interval"
	labelTimeout   = "swarmex.gatekeeper.timeout"
	labelThreshold = "swarmex.gatekeeper.threshold"

	// Traefik labels managed by gatekeeper
	traefikEnable = "traefik.enable"

	defaultPath      = "/health/ready"
	defaultInterval  = 5 * time.Second
	defaultTimeout   = 3 * time.Second
	defaultThreshold = 3
)

// Config parsed from Docker service labels.
type Config struct {
	Path      string
	Interval  time.Duration
	Timeout   time.Duration
	Threshold int
}

// serviceState tracks readiness check state per service.
type serviceState struct {
	config     Config
	successes  int
	ready      bool
	cancelFunc context.CancelFunc
}

// Gatekeeper watches Docker services and gates Traefik routing based on L7 readiness.
type Gatekeeper struct {
	client   *client.Client
	logger   *slog.Logger
	services map[string]*serviceState // keyed by service ID
	mu       sync.Mutex
}

// New creates a Gatekeeper.
func New(cli *client.Client, logger *slog.Logger) *Gatekeeper {
	return &Gatekeeper{
		client:   cli,
		logger:   logger,
		services: make(map[string]*serviceState),
	}
}

// HandleEvent processes Docker events. Wire this to the event-controller.
func (g *Gatekeeper) HandleEvent(ctx context.Context, event events.Message) {
	if event.Type != events.ServiceEventType {
		return
	}

	serviceID := event.Actor.ID

	switch event.Action {
	case events.ActionCreate, events.ActionUpdate:
		g.reconcileService(ctx, serviceID)
	case events.ActionRemove:
		g.stopChecking(serviceID)
	}
}

func (g *Gatekeeper) reconcileService(ctx context.Context, serviceID string) {
	svc, _, err := g.client.ServiceInspectWithRaw(ctx, serviceID, types.ServiceInspectOptions{})
	if err != nil {
		g.logger.Error("inspect service failed", "service", serviceID, "error", err)
		return
	}

	labels := svc.Spec.Labels
	if labels[labelEnabled] != "true" {
		g.stopChecking(serviceID)
		return
	}

	cfg := parseConfig(labels)

	g.mu.Lock()
	// Stop existing checker if config changed
	if existing, ok := g.services[serviceID]; ok {
		existing.cancelFunc()
	}

	checkCtx, cancel := context.WithCancel(ctx)
	state := &serviceState{
		config:     cfg,
		cancelFunc: cancel,
	}
	g.services[serviceID] = state
	g.mu.Unlock()

	g.logger.Info("gatekeeper watching service",
		"service", svc.Spec.Name,
		"path", cfg.Path,
		"interval", cfg.Interval,
		"threshold", cfg.Threshold,
	)

	go g.checkLoop(checkCtx, serviceID, svc.Spec.Name)
}

func (g *Gatekeeper) checkLoop(ctx context.Context, serviceID, serviceName string) {
	g.mu.Lock()
	state := g.services[serviceID]
	g.mu.Unlock()
	if state == nil {
		return
	}

	ticker := time.NewTicker(state.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ok := g.probeService(ctx, serviceID, state.config)
			g.mu.Lock()
			if ok {
				state.successes++
			} else {
				state.successes = 0
			}

			wasReady := state.ready
			state.ready = state.successes >= state.config.Threshold

			if state.ready != wasReady {
				if state.ready {
					g.logger.Info("service READY, enabling Traefik", "service", serviceName)
					g.setTraefikLabel(ctx, serviceID, "true")
				} else {
					g.logger.Warn("service NOT READY, disabling Traefik", "service", serviceName)
					g.setTraefikLabel(ctx, serviceID, "false")
				}
			}
			g.mu.Unlock()

		case <-ctx.Done():
			return
		}
	}
}

func (g *Gatekeeper) probeService(ctx context.Context, serviceID string, cfg Config) bool {
	// Get tasks for this service to find a running container IP
	tasks, err := g.client.TaskList(ctx, types.TaskListOptions{
		Filters: serviceFilter(serviceID),
	})
	if err != nil || len(tasks) == 0 {
		return false
	}

	// Check first running task
	for _, task := range tasks {
		if task.Status.State != swarm.TaskStateRunning {
			continue
		}
		for _, att := range task.NetworksAttachments {
			for _, addr := range att.Addresses {
				ip := stripCIDR(addr)
				url := fmt.Sprintf("http://%s%s", ip, cfg.Path)

				httpClient := &http.Client{Timeout: cfg.Timeout}
				resp, err := httpClient.Get(url)
				if err != nil {
					return false
				}
				resp.Body.Close()
				return resp.StatusCode == http.StatusOK
			}
		}
	}
	return false
}

func (g *Gatekeeper) setTraefikLabel(ctx context.Context, serviceID, value string) {
	svc, _, err := g.client.ServiceInspectWithRaw(ctx, serviceID, types.ServiceInspectOptions{})
	if err != nil {
		g.logger.Error("inspect for label update failed", "service", serviceID, "error", err)
		return
	}

	if svc.Spec.Labels == nil {
		svc.Spec.Labels = make(map[string]string)
	}
	svc.Spec.Labels[traefikEnable] = value

	_, err = g.client.ServiceUpdate(ctx, serviceID, svc.Version, svc.Spec, types.ServiceUpdateOptions{})
	if err != nil {
		g.logger.Error("service update failed", "service", serviceID, "error", err)
	}
}

func (g *Gatekeeper) stopChecking(serviceID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if state, ok := g.services[serviceID]; ok {
		state.cancelFunc()
		delete(g.services, serviceID)
	}
}

func parseConfig(labels map[string]string) Config {
	cfg := Config{
		Path:      defaultPath,
		Interval:  defaultInterval,
		Timeout:   defaultTimeout,
		Threshold: defaultThreshold,
	}
	if v, ok := labels[labelPath]; ok {
		cfg.Path = v
	}
	if v, ok := labels[labelInterval]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Interval = d
		}
	}
	if v, ok := labels[labelTimeout]; ok {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Timeout = d
		}
	}
	if v, ok := labels[labelThreshold]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Threshold = n
		}
	}
	return cfg
}

func stripCIDR(addr string) string {
	for i, c := range addr {
		if c == '/' {
			return addr[:i]
		}
	}
	return addr
}
