package manager

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"text/template"

	"github.com/dominikbraun/graph"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	"mvdan.cc/sh/v3/syntax"

	"github.com/tjhop/mango/internal/inventory"
	"github.com/tjhop/mango/internal/shell"
)

type contextKey string

func (c contextKey) String() string {
	return "mango manager context key " + string(c)
}

var (
	// context keys for metadata that manager uses for feeding mango metadata in templates
	contextKeyRunID         = contextKey("runID")
	contextKeyEnrolled      = contextKey("enrolled")
	contextKeyManagerName   = contextKey("manager_name")
	contextKeyInventoryPath = contextKey("inventory_path")
	contextKeyHostname      = contextKey("hostname")
)

// Manager contains fields related to track and execute runnable modules and statistics.
type Manager struct {
	id            string
	inv           inventory.Store // TODO: move this interface to be defined consumer-side in manager vs in inventory
	modules       graph.Graph[string, Module]
	directives    []Directive
	hostVariables VariableSlice
	runLock       sync.Mutex
	funcMap       template.FuncMap
	tmplData      templateData
}

func (mgr *Manager) String() string { return mgr.id }

// NewManager returns a new Manager struct instantiated with the given ID
func NewManager(id string) *Manager {
	funcs := template.FuncMap{
		"isIPv4":         isIPv4,
		"isIPv6":         isIPv6,
		"humanizeBytes":  humanizeBytes,
		"humanizeIBytes": humanizeIBytes,
	}

	return &Manager{
		id:      id,
		funcMap: funcs,
		modules: graph.New(moduleHash, graph.Directed(), graph.Acyclic()),
	}
}

func getOrSetRunID(ctx context.Context) (context.Context, ulid.ULID) {
	id := ctx.Value(contextKeyRunID)

	if id == nil || id.(ulid.ULID).String() == "" {
		id = ulid.Make()
		ctx = context.WithValue(ctx, contextKeyRunID, id)
	}

	return ctx, id.(ulid.ULID)
}

// ReloadAndRunAll is a wrapper function to reload from the specified
// inventory, populate some run specific context, and initiate a run of all
// managed modules
func (mgr *Manager) ReloadAndRunAll(ctx context.Context, logger *slog.Logger, inv inventory.Store) {
	// add context data relevant to this run, for use with templating and things
	ctx, runID := getOrSetRunID(ctx)

	enrolled := inv.IsEnrolled()
	ctx = context.WithValue(ctx, contextKeyEnrolled, enrolled)
	ctx = context.WithValue(ctx, contextKeyManagerName, mgr.String())
	ctx = context.WithValue(ctx, contextKeyInventoryPath, inv.GetInventoryPath())
	ctx = context.WithValue(ctx, contextKeyHostname, inv.GetHostname())

	logger = logger.With(
		slog.Group(
			"manager",
			// TODO: verify these log keys don't include contextKey prefix
			slog.Bool(string(contextKeyEnrolled), enrolled),
			slog.String(string(contextKeyRunID), runID.String()),
		),
	)

	mgr.Reload(ctx, logger, inv)
	mgr.RunAll(ctx, logger)
}

// Reload accepts a struct that fulfills the inventory.Store interface and
// reloads the hosts modules/directives from the inventory
func (mgr *Manager) Reload(ctx context.Context, logger *slog.Logger, inv inventory.Store) {
	ctx, _ = getOrSetRunID(ctx)

	// reload manager's knowledge of system info
	mgr.tmplData.OS = getOSMetadata(ctx, logger)
	mgr.tmplData.Kernel = getKernelMetadata(ctx, logger)
	mgr.tmplData.CPU = getCPUMetadata(ctx, logger)
	mgr.tmplData.Memory = getMemoryMetadata(ctx, logger)
	mgr.tmplData.Storage = getStorageMetadata(ctx, logger)

	// reload manager's copy of inventory from provided inventory
	logger.InfoContext(ctx, "Reloading items from inventory")

	mgr.inv = inv
	// reload modules
	mgr.ReloadModules(ctx, logger)

	// reload directives
	mgr.ReloadDirectives(ctx)

	// ensure vars are only sourced on manager reload, to avoid needlessly
	// sourcing variables potentially multiple times during a run (which is
	// triggered directly after a reload of data from inventory)
	hostVarsPaths := inv.GetVariablesForSelf()
	if len(hostVarsPaths) > 0 {
		mgr.hostVariables = mgr.ReloadVariables(ctx, logger, hostVarsPaths, nil)
	} else {
		logger.DebugContext(ctx, "No host variables")
	}
}

func (mgr *Manager) ReloadVariables(ctx context.Context, logger *slog.Logger, paths []string, hostVars VariableMap) VariableSlice {
	var varMaps []VariableMap

	for _, path := range paths {
		allTemplateData := mgr.getTemplateData(ctx, path, hostVars, nil, hostVars)
		renderedVars, err := templateScript(ctx, path, allTemplateData, mgr.funcMap)
		if err != nil {
			logger.LogAttrs(
				ctx,
				slog.LevelError,
				"Failed to template variables",
				slog.String("err", err.Error()),
				slog.String("path", path),
			)
			return nil
		}

		// source variables from the templated variables file
		file, err := syntax.NewParser().Parse(strings.NewReader(renderedVars), "")
		if err != nil {
			logger.LogAttrs(
				ctx,
				slog.LevelError,
				"Failed to parse variables",
				slog.String("err", err.Error()),
				slog.String("path", path),
			)
			return nil
		}

		vars, err := shell.SourceNode(ctx, file)
		if err != nil {
			logger.LogAttrs(
				ctx,
				slog.LevelError,
				"Failed to reload variables",
				slog.String("err", err.Error()),
				slog.String("path", path),
			)
			return nil
		}

		varMaps = append(varMaps, shell.MakeVariableMap(vars))
	}

	return shell.MergeVariables(varMaps...)
}

// RunAll runs all of the Directives being managed by the Manager, followed by
// all of the Modules being managed by the Manager.
func (mgr *Manager) RunAll(ctx context.Context, logger *slog.Logger) {
	ctx, _ = getOrSetRunID(ctx)

	go func() {
		logger.InfoContext(ctx, "Run started")
		metricManagerRunInProgress.With(prometheus.Labels{"manager": mgr.String()}).Set(1)

		defer func() {
			metricManagerRunInProgress.With(prometheus.Labels{"manager": mgr.String()}).Set(0)
			logger.InfoContext(ctx, "Run rinished")
		}()

		if !mgr.runLock.TryLock() {
			logger.WarnContext(ctx, "Manager run already in progress, aborting")
			return
		}
		defer mgr.runLock.Unlock()

		directiveLogger := logger.With(
			slog.String("runner", "directives"),
		)
		mgr.RunDirectives(ctx, directiveLogger)
		moduleLogger := logger.With(
			slog.String("runner", "modules"),
		)
		mgr.RunModules(ctx, moduleLogger)
	}()
}
