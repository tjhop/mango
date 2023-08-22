package manager

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/dominikbraun/graph"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"github.com/tjhop/mango/internal/inventory"
	"github.com/tjhop/mango/internal/shell"
	"github.com/tjhop/mango/internal/utils"
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
func (mgr *Manager) ReloadModules(ctx context.Context) {
	// get all modules from inventory applicable to this system
	rawMods := mgr.inv.GetModulesForSelf()
	modGraph := graph.New(moduleHash, graph.Directed(), graph.PreventCycles())
	for _, mod := range rawMods {
		newMod := Module{m: mod}

		// if the module has a variables file set, source it and store
		// the expanded variables
		if mod.Variables != "" {
			newMod.Variables = mgr.ReloadVariables(ctx, []string{mod.Variables}, shell.MakeVariableMap(mgr.hostVariables))
		} else {
			mgr.logger.Debug("No module variables")
		}

		err := modGraph.AddVertex(newMod)
		if err != nil {
			mgr.logger.WithFields(log.Fields{
				"module": mod.String(),
				"error":  err,
			}).Error("Failed to add module to DAG")
		}
	}

	for _, mod := range rawMods {
		// if the module has a requirements file set, parse it line by
		// line and add edges to the graph for ordering
		if mod.Requires == "" {
			mgr.logger.Debug("No module variables")
			continue
		}

		lines := utils.ReadFileLines(mod.Requires)
		for line := range lines {
			if line.Err != nil {
				log.WithFields(log.Fields{
					"path":  mod.Requires,
					"error": line.Err,
				}).Error("Failed to read requirements for this module")
			} else {
				err := modGraph.AddEdge(filepath.Join(mgr.inv.GetInventoryPath(), "modules", line.Text), mod.ID)
				if err != nil {
					mgr.logger.WithFields(log.Fields{
						"module": mod.String(),
						"error":  err,
					}).Error("Failed to add module to DAG")
				}
			}
		}
	}

	mgr.modules = modGraph
}

// RunModule is responsible for actually executing a module, using the `shell`
// package.
func (mgr *Manager) RunModule(ctx context.Context, mod Module) error {
	runID := ctx.Value(contextKeyRunID).(ulid.ULID)
	logger := mgr.logger.WithFields(log.Fields{
		"run_id": runID.String(),
	})

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
		logger.WithFields(log.Fields{
			"module": mod.String(),
		}).Warn("Module has no test script, proceeding to apply")
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
			logger.WithFields(log.Fields{
				"module": mod.m.Test,
			}).Warn("Failed module test, running apply to get system to desired state")
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
func (mgr *Manager) RunModules(ctx context.Context) {
	runID := ctx.Value(contextKeyRunID).(ulid.ULID)
	logger := mgr.logger.WithFields(log.Fields{
		"run_id": runID.String(),
	})

	logger.Info("Module run started")
	defer logger.Info("Module run finished")

	order, err := graph.TopologicalSort(mgr.modules)
	if err != nil {
		mgr.logger.WithFields(log.Fields{
			"error": err,
		}).Error("Failed to sort DAG")
	}

	if len(order) <= 0 {
		logger.Info("No Modules to run")
		return
	}

	for _, v := range order {
		mod, err := mgr.modules.Vertex(v)
		if err != nil {
			logger.WithFields(log.Fields{
				"module": mod.String(),
				"error":  err,
			}).Error("Module failed")
		}

		logger.WithFields(log.Fields{
			"module": mod.String(),
		}).Info("Running Module")

		if err := mgr.RunModule(ctx, mod); err != nil {
			logger.WithFields(log.Fields{
				"module": mod.String(),
				"error":  err,
			}).Error("Module failed")
		}
	}
}
