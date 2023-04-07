package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	log "github.com/sirupsen/logrus"

	"github.com/tjhop/mango/internal/inventory"
	"github.com/tjhop/mango/internal/shell"
)

var (
	// prometheus metrics

	// module run stat metrics
	metricManagerModuleRunTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_module_run_seconds",
			Help: "Timestamp of the last run of the given module, in seconds since the epoch",
		},
		[]string{"module", "script"},
	)

	metricManagerModuleRunSuccessTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_module_run_success_seconds",
			Help: "Timestamp of the last successful run of the given module, in seconds since the epoch",
		},
		[]string{"module", "script"},
	)

	metricManagerModuleRunDuration = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_module_run_duration_seconds",
			Help: "Approximately how long it took for the module to run, in seconds",
		},
		[]string{"module", "script"},
	)

	metricManagerModuleRunTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_module_run_total",
			Help: "A count of the total number of runs that have been performed to manage the module",
		},
		[]string{"module", "script"},
	)

	metricManagerModuleRunFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_module_run_failed_total",
			Help: "A count of the total number of failed runs that have been performed to manage the module",
		},
		[]string{"module", "script"},
	)

	// directive run stat metrics
	metricManagerDirectiveRunTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_directive_run_seconds",
			Help: "Timestamp of the last run of the given directive, in seconds since the epoch",
		},
		[]string{"directive"},
	)

	metricManagerDirectiveRunSuccessTimestamp = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_directive_run_success_seconds",
			Help: "Timestamp of the last successful run of the given directive, in seconds since the epoch",
		},
		[]string{"directive"},
	)

	metricManagerDirectiveRunDuration = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_directive_run_duration_seconds",
			Help: "Approximately how long it took for the directive to run, in seconds",
		},
		[]string{"directive"},
	)

	metricManagerDirectiveRunTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_directive_run_total",
			Help: "A count of the total number of runs that have been performed to manage the directive",
		},
		[]string{"directive"},
	)

	metricManagerDirectiveRunFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mango_manager_directive_run_failed_total",
			Help: "A count of the total number of failed runs that have been performed to manage the directive",
		},
		[]string{"directive"},
	)
)

// Module is a wrapper struct that encapsulates an inventory.Module, and
// exports a `Variables`, which is a `shell.VariableMap` of the expanded
// variables for the given module
type Module struct {
	m         inventory.Module
	Variables shell.VariableMap
}

func (mod Module) String() string { return mod.m.String() }

// Directive is a wrapper struct that encapsulates an inventory.Directive
type Directive struct {
	d inventory.Directive
}

func (dir Directive) String() string { return dir.d.String() }

// Manager contains fields related to track and execute runnable modules and statistics.
type Manager struct {
	id            string
	logger        *log.Entry
	modules       []Module
	directives    []Directive
	hostVariables shell.VariableMap
}

func (mgr *Manager) String() string { return mgr.id }

// NewManager returns a new Manager struct instantiated with the given ID
func NewManager(id string) *Manager {
	return &Manager{
		id: id,
		logger: log.WithFields(log.Fields{
			"manager": id,
		}),
	}
}

// ReloadDirectives reloads the manager's directives from the specified inventory.
func (mgr *Manager) ReloadDirectives(ctx context.Context, inv inventory.Store) {
	// get all directives (directives are applied to all systems if modtime threshold is passed)
	rawDirScripts := inv.GetDirectivesForSelf()
	dirScripts := make([]Directive, len(rawDirScripts))
	for i, ds := range rawDirScripts {
		dirScripts[i] = Directive{d: ds}
	}

	mgr.directives = dirScripts
}

// ReloadModules reloads the manager's modules from the specified inventory.
func (mgr *Manager) ReloadModules(ctx context.Context, inv inventory.Store) {
	// get all modules from inventory applicable to this system
	rawMods := inv.GetModulesForSelf()
	modules := make([]Module, len(rawMods))
	for i, mod := range rawMods {
		newMod := Module{m: mod}
		newModVars := make(shell.VariableMap)

		// if the module has a variables file set, source it and store
		// the expanded variables
		if mod.Variables != "" {
			vars, err := shell.SourceFile(ctx, mod.Variables)
			if err != nil {
				mgr.logger.WithFields(log.Fields{
					"error": err,
				}).Error("Failed to expand module variables")
			} else {
				var varKeys []string
				for k := range vars {
					varKeys = append(varKeys, k)
				}
				mgr.logger.WithFields(log.Fields{
					"variables": varKeys,
				}).Debug("Expanding module variables")
				newModVars = vars
			}
		} else {
			mgr.logger.Debug("No module variables")
		}

		newMod.Variables = newModVars
		modules[i] = newMod
	}

	mgr.modules = modules
}

// Reload accepts a struct that fulfills the inventory.Store interface and
// reloads the hosts modules/directives from the inventory
func (mgr *Manager) Reload(ctx context.Context, inv inventory.Store) {
	mgr.logger.Info("Reloading items from inventory")

	// reload modules
	mgr.ReloadModules(ctx, inv)

	// reload directives
	mgr.ReloadDirectives(ctx, inv)

	// ensure vars are only on manager reload, to avoid needlessly sourcing
	// variables potentially multiple times during a run (which is
	// triggered directly after a reload of data from inventory)
	hostVarsPath := inv.GetVariablesForSelf()
	if hostVarsPath != "" {
		vars, err := shell.SourceFile(ctx, hostVarsPath)
		if err != nil {
			mgr.logger.WithFields(log.Fields{
				"path": hostVarsPath,
			}).Error("Failed to reload host variables")
		} else {
			varKeys := make([]string, len(vars))
			for k := range vars {
				varKeys = append(varKeys, k)
			}
			mgr.logger.WithFields(log.Fields{
				"variables": varKeys,
			}).Debug("Reloading host variables")
			mgr.hostVariables = vars
		}
	} else {
		mgr.logger.Debug("No host variables")
	}
}

// RunDirective is responsible for actually executing a module, using the `shell`
// package.
func (mgr *Manager) RunDirective(ctx context.Context, ds Directive) error {
	applyStart := time.Now()
	labels := prometheus.Labels{
		"directive": ds.String(),
	}
	metricManagerDirectiveRunTimestamp.With(labels).Set(float64(applyStart.Unix()))

	// TODO: are host vars allowed in directives?
	err := shell.Run(ctx, ds.String(), nil, nil)

	// update metrics regardless of error, so do them before handling error
	applyEnd := time.Since(applyStart)
	metricManagerDirectiveRunSuccessTimestamp.With(labels).Set(float64(applyStart.Unix()))
	metricManagerDirectiveRunDuration.With(labels).Set(float64(applyEnd))
	metricManagerDirectiveRunTotal.With(labels).Inc()

	if err != nil {
		metricManagerDirectiveRunFailedTotal.With(labels).Inc()
		return fmt.Errorf("Failed to apply directive: %v", err)
	}

	return nil
}

// RunDirectives runs all of the directive scripts being managed by the Manager
func (mgr *Manager) RunDirectives(ctx context.Context) {
	if len(mgr.directives) <= 0 {
		mgr.logger.Info("No Directives to run")
		return
	}

	defer mgr.logger.Info("All directives have been run")
	for _, d := range mgr.directives {
		mgr.logger.WithFields(log.Fields{
			"directive": d.String(),
		}).Info("Running directive")

		if err := mgr.RunDirective(ctx, d); err != nil {
			mgr.logger.WithFields(log.Fields{
				"directive": d.String(),
				"error":     err,
			}).Error("Directive failed")
		}
	}
}

// RunModule is responsible for actually executing a module, using the `shell`
// package.
func (mgr *Manager) RunModule(ctx context.Context, mod Module) error {
	if mod.m.Apply == "" {
		// TODO: convert to const errs?
		return fmt.Errorf("Module has no apply script")
	}

	labels := prometheus.Labels{
		"module": mod.String(),
	}

	if mod.m.Test == "" {
		mgr.logger.WithFields(log.Fields{
			"module": mod.String(),
		}).Warn("Module has no test script, proceeding to apply")
	} else {
		testStart := time.Now()
		labels["script"] = "test"
		metricManagerModuleRunTimestamp.With(labels).Set(float64(testStart.Unix()))

		if err := shell.Run(ctx, mod.m.Test, mgr.hostVariables, mod.Variables); err != nil {
			// if test script for a module fails, log a warning for user and continue with apply
			metricManagerModuleRunFailedTotal.With(labels).Inc()
			mgr.logger.WithFields(log.Fields{
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

	err := shell.Run(ctx, mod.m.Apply, mgr.hostVariables, mod.Variables)

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
	if len(mgr.modules) <= 0 {
		mgr.logger.Info("No Modules to run")
		return
	}

	defer mgr.logger.Info("All modules have been run")
	for _, mod := range mgr.modules {
		mgr.logger.WithFields(log.Fields{
			"module": mod.String(),
		}).Info("Running Module")

		if err := mgr.RunModule(ctx, mod); err != nil {
			mgr.logger.WithFields(log.Fields{
				"module": mod.String(),
				"error":  err,
			}).Error("Module failed")
		}
	}
}

// RunAll runs all of the Directives being managed by the Manager, followed by
// all of the Modules being managed by the Manager.
func (mgr *Manager) RunAll(ctx context.Context) {
	mgr.RunDirectives(ctx)
	mgr.RunModules(ctx)
}
