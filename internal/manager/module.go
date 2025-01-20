package manager

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/dominikbraun/graph"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"

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
		modLogger := logger.With(
			slog.Group(
				"module",
				slog.String("id", mod.String()),
			),
		)

		// if the module has a variables file set, source it and store
		// the expanded variables
		if mod.Variables != "" {
			newMod.Variables = mgr.ReloadVariables(ctx, modLogger, []string{mod.Variables}, shell.MakeVariableMap(mgr.hostVariables), mgr.hostTemplates)
		} else {
			modLogger.DebugContext(ctx, "No module variables")
		}

		err := modGraph.AddVertex(newMod)
		if err != nil {
			modLogger.LogAttrs(
				ctx,
				slog.LevelError,
				"Failed to add module to directed acyclic graph",
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
						"Failed to add module dependency as edge to directed acyclic graph",
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
		"script": "",
	}

	hostVarsMap := shell.MakeVariableMap(mgr.hostVariables)
	modVarsMap := shell.MakeVariableMap(mod.Variables)
	allVars := shell.MergeVariables(hostVarsMap, modVarsMap)
	allVarsMap := shell.MakeVariableMap(allVars)
	allTemplateData := mgr.getTemplateData(ctx, mod.String(), hostVarsMap, modVarsMap, allVarsMap)
	allUserTemplateFiles := append(mgr.hostTemplates, mod.m.TemplateFiles...)

	var testRC uint8
	if mod.m.Test == "" {
		logger.LogAttrs(
			ctx,
			slog.LevelWarn,
			"Module has no test script, proceeding to apply",
		)
	} else {
		testStart := time.Now()
		labels["script"] = "test"
		metricManagerModuleRunTimestamp.With(labels).Set(float64(testStart.Unix()))

		renderedTest, err := templateScript(ctx, mod.m.Test, allTemplateData, mgr.funcMap, allUserTemplateFiles...)
		if err != nil {
			return fmt.Errorf("Failed to template script: %s", err)
		}

		testRC, err = shell.Run(ctx, runID, mod.m.Test, renderedTest, allVars)
		// update metrics regardless of error, so do them before handling error
		metricManagerModuleRunDuration.With(labels).Observe(float64(time.Since(testStart).Seconds()))
		metricManagerModuleRunTotal.With(labels).Inc()
		switch {
		case err != nil:
			// if test script for a module fails, log a warning for user and continue with apply
			metricManagerModuleRunFailedTotal.With(labels).Inc()
			logger.LogAttrs(
				ctx,
				slog.LevelWarn,
				"Failed to run module test",
				slog.String("err", err.Error()),
			)
		case testRC != 0:
			// if test script for a module fails, log a warning for user and continue with apply
			metricManagerModuleRunFailedTotal.With(labels).Inc()
			logger.LogAttrs(
				ctx,
				slog.LevelWarn,
				"Failed to run module test, received non-zero exit code",
				slog.Any("exit_code", testRC),
			)
		default:
			metricManagerModuleRunSuccessTimestamp.With(labels).Set(float64(testStart.Unix()))
		}
	}

	if viper.GetBool("manager.skip-apply-on-test-success") && mod.m.Test != "" && testRC == 0 {
		logger.LogAttrs(
			ctx,
			slog.LevelDebug,
			"Skipping module apply script because test script ran successfully and mango has been started with flag `--manager.skip-apply-on-test-success`",
		)

		return nil
	}

	applyStart := time.Now()
	labels["script"] = "apply"
	metricManagerModuleRunTimestamp.With(labels).Set(float64(applyStart.Unix()))

	renderedApply, err := templateScript(ctx, mod.m.Apply, allTemplateData, mgr.funcMap, allUserTemplateFiles...)
	if err != nil {
		return fmt.Errorf("Failed to template script: %s", err)
	}

	applyRC, err := shell.Run(ctx, runID, mod.m.Apply, renderedApply, allVars)
	// update metrics regardless of error, so do them before handling error
	metricManagerModuleRunDuration.With(labels).Observe(float64(time.Since(applyStart).Seconds()))
	metricManagerModuleRunTotal.With(labels).Inc()
	switch {
	case err != nil:
		metricManagerModuleRunFailedTotal.With(labels).Inc()
		return fmt.Errorf("Failed to run module apply: %v", err)
	case applyRC != 0:
		// if apply script for a module fails, log a warning for user and continue with apply
		metricManagerModuleRunFailedTotal.With(labels).Inc()
		return fmt.Errorf("Failed to run module apply, non-zero exit code returned: %d", applyRC)
	default:
		metricManagerModuleRunSuccessTimestamp.With(labels).Set(float64(applyStart.Unix()))
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
			"Failed to sort directed acyclic graph",
			slog.String("err", err.Error()),
		)
	}

	if len(order) <= 0 {
		logger.InfoContext(ctx, "No Modules to run")
		return
	}

	for _, v := range order {
		vLogger := logger.With(
			slog.Group(
				"module",
				slog.String("id", v),
			),
		)

		mod, err := mgr.modules.Vertex(v)
		if err != nil {
			vLogger.LogAttrs(
				ctx,
				slog.LevelError,
				"Failed to retrieve module from directed acyclic graph vertex",
				slog.String("err", err.Error()),
			)
		}

		vLogger.InfoContext(ctx, "Module started")
		defer vLogger.InfoContext(ctx, "Module finished")

		if err := mgr.RunModule(ctx, vLogger, mod); err != nil {
			vLogger.LogAttrs(
				ctx,
				slog.LevelError,
				"Module failed",
				slog.String("err", err.Error()),
			)
		}
	}
}
