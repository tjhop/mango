package manager

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/dominikbraun/graph"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/tjhop/mango/internal/inventory"
	"github.com/tjhop/mango/internal/shell"
	"github.com/tjhop/mango/pkg/utils"
)

// Module is a wrapper struct that encapsulates an inventory.Module, and
// exports a `Variables`, which a `VariableSlce`, where each item is a variable for
// the module in `key=value` form (the same as returned by `os.Environ()`)
type Module struct {
	m         inventory.Module
	Variables VariableSlice
}

func (mod Module) String() string { return mod.m.String() }

// moduleHash is the hash function used to set up the directed acyclic graph
// for module ordering/dependency management
var moduleHash = func(m Module) string {
	return m.String()
}

// ReloadModules reloads the manager's modules from the specified inventory.
func (mgr *Manager) ReloadModules(ctx context.Context, logger *slog.Logger) {
	ctx, _ = getOrSetRunID(ctx)

	// get all modules from inventory applicable to this system
	rawMods := mgr.inv.GetModulesForSelf()

	// add all modules as vertices in DAG. this must be done first before
	// attempting to set any edges for requirements, so that we're sure the
	// vertices already exist
	modGraph := graph.New(moduleHash, graph.Directed(), graph.PreventCycles())
	for _, mod := range rawMods {
		newMod := Module{m: mod}
		logger = logger.With(
			slog.Group(
				"module",
				slog.String("id", mod.String()),
			),
		)

		// if the module has a variables file set, source it and store
		// the expanded variables
		if mod.Variables != "" {
			newMod.Variables = mgr.ReloadVariables(ctx, logger, []string{mod.Variables}, shell.MakeVariableMap(mgr.hostVariables))
		} else {
			logger.DebugContext(ctx, "No module variables")
		}

		err := modGraph.AddVertex(newMod)
		if err != nil {
			logger.LogAttrs(
				ctx,
				slog.LevelError,
				"Failed to add module to DAG",
				slog.String("err", err.Error()),
			)
		}
	}

	// add any module requirement files as edges between the vertices(modules) in the DAG
	for _, mod := range rawMods {
		// if the module has a requirements file set, parse it line by
		// line and add edges to the graph for ordering
		if mod.Requires == "" {
			logger.DebugContext(ctx, "No module variables")
			continue
		}

		lines := utils.ReadFileLines(mod.Requires)
		for line := range lines {
			if line.Err != nil {
				logger.LogAttrs(
					ctx,
					slog.LevelError,
					"Failed to read requirements for this module",
					slog.String("err", line.Err.Error()),
					slog.String("path", mod.Requires),
				)
			} else {
				err := modGraph.AddEdge(filepath.Join(mgr.inv.GetInventoryPath(), "modules", line.Text), mod.ID)
				if err != nil {
					logger.LogAttrs(
						ctx,
						slog.LevelError,
						"Failed to add module to DAG",
						slog.String("err", err.Error()),
					)
				}
			}
		}
	}

	mgr.modules = modGraph
}

// RunModule is responsible for actually executing a module, using the `shell`
// package.
func (mgr *Manager) RunModule(ctx context.Context, logger *slog.Logger, mod Module) error {
	ctx, runID := getOrSetRunID(ctx)

	if mod.m.Apply == "" {
		return fmt.Errorf("Module has no apply script")
	}

	labels := prometheus.Labels{
		"module": mod.String(),
	}

	hostVarsMap := shell.MakeVariableMap(mgr.hostVariables)
	modVarsMap := shell.MakeVariableMap(mod.Variables)
	allVars := shell.MergeVariables(hostVarsMap, modVarsMap)
	allVarsMap := shell.MakeVariableMap(allVars)
	allTemplateData := mgr.getTemplateData(ctx, mod.String(), hostVarsMap, modVarsMap, allVarsMap)

	if mod.m.Test == "" {
		logger.LogAttrs(
			ctx,
			slog.LevelWarn,
			"Module has no test script, proceeding to apply",
			slog.String("module", mod.String()),
		)
	} else {
		testStart := time.Now()
		labels["script"] = "test"
		metricManagerModuleRunTimestamp.With(labels).Set(float64(testStart.Unix()))

		renderedTest, err := templateScript(ctx, mod.m.Test, allTemplateData, mgr.funcMap)
		if err != nil {
			return fmt.Errorf("Failed to template script: %s", err)
		}

		if err := shell.Run(ctx, runID, mod.m.Test, renderedTest, allVars); err != nil {
			// if test script for a module fails, log a warning for user and continue with apply
			metricManagerModuleRunFailedTotal.With(labels).Inc()
			logger.WarnContext(ctx, "Failed module test, running apply to get system to desired state")
		} else {
			metricManagerModuleRunTotal.With(labels).Inc()
			metricManagerModuleRunSuccessTimestamp.With(labels).Set(float64(testStart.Unix()))
		}

		testEnd := time.Since(testStart)
		metricManagerModuleRunDuration.With(labels).Set(float64(testEnd))
	}

	applyStart := time.Now()
	labels["script"] = "apply"
	metricManagerModuleRunTimestamp.With(labels).Set(float64(applyStart.Unix()))

	renderedApply, err := templateScript(ctx, mod.m.Apply, allTemplateData, mgr.funcMap)
	if err != nil {
		return fmt.Errorf("Failed to template script: %s", err)
	}

	err = shell.Run(ctx, runID, mod.m.Apply, renderedApply, allVars)

	// update metrics regardless of error, so do them before handling error
	applyEnd := time.Since(applyStart)
	metricManagerModuleRunSuccessTimestamp.With(labels).Set(float64(applyStart.Unix()))
	metricManagerModuleRunDuration.With(labels).Set(float64(applyEnd))
	metricManagerModuleRunTotal.With(labels).Inc()

	if err != nil {
		metricManagerModuleRunFailedTotal.With(labels).Inc()
		return fmt.Errorf("Failed to apply module: %v", err)
	}

	return nil
}

// RunModules runs all of the modules being managed by the Manager
func (mgr *Manager) RunModules(ctx context.Context, logger *slog.Logger) {
	ctx, _ = getOrSetRunID(ctx)

	logger.InfoContext(ctx, "Module run started")
	defer logger.InfoContext(ctx, "Module run finished")

	order, err := graph.TopologicalSort(mgr.modules)
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to sort DAG",
			slog.String("err", err.Error()),
		)
	}

	if len(order) <= 0 {
		logger.InfoContext(ctx, "No Modules to run")
		return
	}

	for _, v := range order {
		logger = logger.With(
			slog.Group(
				"module",
				slog.String("id", v),
			),
		)

		mod, err := mgr.modules.Vertex(v)
		if err != nil {
			logger.LogAttrs(
				ctx,
				slog.LevelError,
				"Failed to retreive module from DAG vertex",
				slog.String("err", err.Error()),
			)
		}

		logger.InfoContext(ctx, "Module started")
		defer logger.InfoContext(ctx, "Module finished")

		if err := mgr.RunModule(ctx, logger, mod); err != nil {
			logger.LogAttrs(
				ctx,
				slog.LevelError,
				"Module failed",
				slog.String("err", err.Error()),
			)
		}
	}
}
