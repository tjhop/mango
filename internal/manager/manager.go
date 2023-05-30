package manager

import (
	"context"
	"fmt"
	"os"
	"sync"
	"text/template"
	"time"

	kernelParser "github.com/moby/moby/pkg/parsers/kernel"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	distro "github.com/quay/claircore/osrelease"
	log "github.com/sirupsen/logrus"

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
	contextKeyManagerName   = contextKey("manager name")
	contextKeyInventoryPath = contextKey("inventory path")
	contextKeyHostname      = contextKey("hostname")

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

	// don't add runID to run-in-progress metric -- even though it could be
	// useful, it'll hurt cardinality. Consider adding it later as a
	// trace/examplar.
	metricManagerRunInProgress = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mango_manager_run_in_progress",
			Help: "A metric with a constant '1' when the named manager is actively running directives/modules",
		},
		[]string{"manager"},
	)
)

// Module is a wrapper struct that encapsulates an inventory.Module, and
// exports a `Variables`, which a `VariableSlce`, where each item is a variable for
// the module in `key=value` form (the same as returned by `os.Environ()`)
type Module struct {
	m         inventory.Module
	Variables VariableSlice
}

func (mod Module) String() string { return mod.m.String() }

// Directive is a wrapper struct that encapsulates an inventory.Directive
type Directive struct {
	d inventory.Directive
}

func (dir Directive) String() string { return dir.d.String() }

// Manager contains fields related to track and execute runnable modules and statistics.
// TODO: manager will eventually hold at least the func map for templates
type Manager struct {
	id            string
	inv           inventory.Store // TODO: move this interface to be defined consumer-side in manager vs in inventory
	logger        *log.Entry
	modules       []Module
	directives    []Directive
	hostVariables VariableSlice
	runLock       sync.Mutex
	funcMap       template.FuncMap
}

func (mgr *Manager) String() string { return mgr.id }

// NewManager returns a new Manager struct instantiated with the given ID
func NewManager(id string) *Manager {
	funcs := template.FuncMap{
		"isIPv4": isIPv4,
		"isIPv6": isIPv6,
	}

	return &Manager{
		id: id,
		logger: log.WithFields(log.Fields{
			"manager": id,
		}),
		funcMap: funcs,
	}
}

// ReloadDirectives reloads the manager's directives from the specified inventory.
func (mgr *Manager) ReloadDirectives(ctx context.Context) {
	// get all directives (directives are applied to all systems if modtime threshold is passed)
	rawDirScripts := mgr.inv.GetDirectivesForSelf()
	dirScripts := make([]Directive, len(rawDirScripts))
	for i, ds := range rawDirScripts {
		dirScripts[i] = Directive{d: ds}
	}

	mgr.directives = dirScripts
}

// ReloadModules reloads the manager's modules from the specified inventory.
func (mgr *Manager) ReloadModules(ctx context.Context) {
	// get all modules from inventory applicable to this system
	rawMods := mgr.inv.GetModulesForSelf()
	modules := make([]Module, len(rawMods))
	for i, mod := range rawMods {
		newMod := Module{m: mod}

		// if the module has a variables file set, source it and store
		// the expanded variables
		if mod.Variables != "" {
			vars, err := shell.SourceFile(ctx, mod.Variables)
			if err != nil {
				mgr.logger.WithFields(log.Fields{
					"error": err,
				}).Error("Failed to expand module variables")
			} else {
				newMod.Variables = vars
			}
		} else {
			mgr.logger.Debug("No module variables")
		}

		modules[i] = newMod
	}

	mgr.modules = modules
}

// ReloadAndRunAll is a wrapper function to reload from the specified
// inventory, populate some run specific context, and initiate a run of all
// managed modules
func (mgr *Manager) ReloadAndRunAll(ctx context.Context, inv inventory.Store) {
	mgr.Reload(ctx, inv)

	// add context data relevant to this run, for use with templating and things
	ctx = context.WithValue(ctx, contextKeyEnrolled, inv.IsEnrolled())
	ctx = context.WithValue(ctx, contextKeyManagerName, mgr.String())
	ctx = context.WithValue(ctx, contextKeyInventoryPath, mgr.inv.GetInventoryPath())
	ctx = context.WithValue(ctx, contextKeyHostname, mgr.inv.GetHostname())

	mgr.RunAll(ctx)
}

// Reload accepts a struct that fulfills the inventory.Store interface and
// reloads the hosts modules/directives from the inventory
func (mgr *Manager) Reload(ctx context.Context, inv inventory.Store) {
	mgr.logger.Info("Reloading items from inventory")

	mgr.inv = inv
	// reload modules
	mgr.ReloadModules(ctx)

	// reload directives
	mgr.ReloadDirectives(ctx)

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
			mgr.hostVariables = vars
		}
	} else {
		mgr.logger.Debug("No host variables")
	}
}

// RunDirective is responsible for actually executing a module, using the `shell`
// package.
func (mgr *Manager) RunDirective(ctx context.Context, ds Directive) error {
	runID := ctx.Value(contextKeyRunID).(ulid.ULID)
	applyStart := time.Now()
	labels := prometheus.Labels{
		"directive": ds.String(),
	}
	metricManagerDirectiveRunTimestamp.With(labels).Set(float64(applyStart.Unix()))

	renderedScript, err := templateScript(ctx, ds.String(), templateView{}, mgr.funcMap)
	if err != nil {
		return fmt.Errorf("Failed to template script: %s", err)
	}

	err = shell.Run(ctx, runID, ds.String(), renderedScript, nil)

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
	runID := ctx.Value(contextKeyRunID).(ulid.ULID)
	logger := mgr.logger.WithFields(log.Fields{
		"run_id": runID.String(),
	})

	if len(mgr.directives) <= 0 {
		logger.Info("No Directives to run")
		return
	}

	logger.Info("Directive run started")
	defer logger.Info("Directive run finished")
	for _, d := range mgr.directives {
		logger.WithFields(log.Fields{
			"directive": d.String(),
		}).Info("Running directive")

		if err := mgr.RunDirective(ctx, d); err != nil {
			logger.WithFields(log.Fields{
				"directive": d.String(),
				"error":     err,
			}).Error("Directive failed")
		}
	}
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

	// runtime metadata for templates
	hostVarsMap := shell.MakeVariableMap(mgr.hostVariables)
	modVarsMap := shell.MakeVariableMap(mod.Variables)
	allVars := shell.MergeVariables(hostVarsMap, modVarsMap)
	allVarsMap := shell.MakeVariableMap(allVars)
	runtimeData := metadata{
		ModuleName:    mod.String(),
		RunID:         ctx.Value(contextKeyRunID).(ulid.ULID).String(),
		Enrolled:      ctx.Value(contextKeyEnrolled).(bool),
		ManagerName:   ctx.Value(contextKeyManagerName).(string),
		InventoryPath: ctx.Value(contextKeyInventoryPath).(string),
		Hostname:      ctx.Value(contextKeyHostname).(string),
	}

	// os metadata for templates
	osReleaseFile, err := os.Open(distro.Path)
	if err != nil {
		return fmt.Errorf("Failed to open %s: %s", distro.Path, err)
	}
	osRelease, err := distro.Parse(ctx, osReleaseFile)
	if err != nil {
		return fmt.Errorf("Failed to parse %s: %s", distro.Path, err)
	}
	osData := osMetadata{
		OSRelease: osRelease,
	}

	// kernel metadata for templates
	kernelInfo, err := kernelParser.GetKernelVersion()
	if err != nil {
		return fmt.Errorf("Failed to get kernel info: %s", err)
	}
	kernelData := kernelMetadata{
		Kernel: kernelInfo.Kernel,
		Major:  kernelInfo.Major,
		Minor:  kernelInfo.Minor,
		Flavor: kernelInfo.Flavor,
	}

	// assemble all template data
	allTemplateData := templateView{
		Mango: templateData{
			HostVars:   VariableMap(hostVarsMap),
			ModuleVars: VariableMap(modVarsMap),
			Vars:       VariableMap(allVarsMap),
			Metadata:   runtimeData,
			OS:         osData,
			Kernel:     kernelData,
		},
	}

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

	if len(mgr.modules) <= 0 {
		logger.Info("No Modules to run")
		return
	}

	logger.Info("Module run started")
	defer logger.Info("Module run finished")
	for _, mod := range mgr.modules {
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

// RunAll runs all of the Directives being managed by the Manager, followed by
// all of the Modules being managed by the Manager.
func (mgr *Manager) RunAll(ctx context.Context) {
	go func() {
		metricManagerRunInProgress.With(prometheus.Labels{"manager": mgr.String()}).Set(1)
		defer metricManagerRunInProgress.With(prometheus.Labels{"manager": mgr.String()}).Set(0)
		runID := ulid.Make()
		runCtx := context.WithValue(ctx, contextKeyRunID, runID)
		logger := mgr.logger.WithFields(log.Fields{
			"run_id": runID.String(),
		})

		if !mgr.runLock.TryLock() {
			logger.Warn("Manager run already in progress, aborting")
			return
		}
		defer mgr.runLock.Unlock()

		mgr.RunDirectives(runCtx)
		mgr.RunModules(runCtx)
	}()
}
