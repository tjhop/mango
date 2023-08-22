package manager

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/dominikbraun/graph"
	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"mvdan.cc/sh/v3/syntax"

	"github.com/tjhop/mango/internal/inventory"
	"github.com/tjhop/mango/internal/shell"
	"github.com/tjhop/mango/internal/utils"
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

// Directive is a wrapper struct that encapsulates an inventory.Directive
type Directive struct {
	d inventory.Directive
}

func (dir Directive) String() string { return dir.d.String() }

// Manager contains fields related to track and execute runnable modules and statistics.
type Manager struct {
	id            string
	inv           inventory.Store // TODO: move this interface to be defined consumer-side in manager vs in inventory
	logger        *log.Entry
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
		"isIPv4": isIPv4,
		"isIPv6": isIPv6,
	}

	return &Manager{
		id: id,
		logger: log.WithFields(log.Fields{
			"manager": id,
		}),
		funcMap: funcs,
		modules: graph.New(moduleHash, graph.Directed(), graph.Acyclic()),
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

// ReloadAndRunAll is a wrapper function to reload from the specified
// inventory, populate some run specific context, and initiate a run of all
// managed modules
func (mgr *Manager) ReloadAndRunAll(ctx context.Context, inv inventory.Store) {
	// add context data relevant to this run, for use with templating and things
	runID := ulid.Make()
	ctx = context.WithValue(ctx, contextKeyRunID, runID)
	ctx = context.WithValue(ctx, contextKeyEnrolled, inv.IsEnrolled())
	ctx = context.WithValue(ctx, contextKeyManagerName, mgr.String())
	ctx = context.WithValue(ctx, contextKeyInventoryPath, inv.GetInventoryPath())
	ctx = context.WithValue(ctx, contextKeyHostname, inv.GetHostname())

	mgr.Reload(ctx, inv)
	mgr.RunAll(ctx)
}

// Reload accepts a struct that fulfills the inventory.Store interface and
// reloads the hosts modules/directives from the inventory
func (mgr *Manager) Reload(ctx context.Context, inv inventory.Store) {
	// reload manager's knowledge of system info
	osData, kernelData := getSystemMetadata()
	mgr.tmplData.OS = osData
	mgr.tmplData.Kernel = kernelData

	// reload manager's copy of inventory from provided inventory
	mgr.logger.Info("Reloading items from inventory")

	mgr.inv = inv
	// reload modules
	mgr.ReloadModules(ctx)

	// reload directives
	mgr.ReloadDirectives(ctx)

	// ensure vars are only sourced on manager reload, to avoid needlessly
	// sourcing variables potentially multiple times during a run (which is
	// triggered directly after a reload of data from inventory)
	hostVarsPaths := inv.GetVariablesForSelf()
	if len(hostVarsPaths) > 0 {
		mgr.hostVariables = mgr.ReloadVariables(ctx, hostVarsPaths, nil)
	} else {
		mgr.logger.Debug("No host variables")
	}
}

func (mgr *Manager) ReloadVariables(ctx context.Context, paths []string, hostVars VariableMap) VariableSlice {
	var varMaps []VariableMap

	for _, path := range paths {
		allTemplateData := mgr.getTemplateData(ctx, path, hostVars, nil, hostVars)
		renderedVars, err := templateScript(ctx, path, allTemplateData, mgr.funcMap)
		if err != nil {
			mgr.logger.WithFields(log.Fields{
				"path":  path,
				"error": err,
			}).Error("Failed to template variables")
			return nil
		}

		// source variables from the templated variables file
		file, err := syntax.NewParser().Parse(strings.NewReader(renderedVars), "")
		if err != nil {
			mgr.logger.WithFields(log.Fields{
				"path":  path,
				"error": err,
			}).Error("Failed to parse variables")
			return nil
		}

		vars, err := shell.SourceNode(ctx, file)
		if err != nil {
			mgr.logger.WithFields(log.Fields{
				"path":  path,
				"error": err,
			}).Error("Failed to reload variables")
			return nil
		}

		varMaps = append(varMaps, shell.MakeVariableMap(vars))
	}

	return shell.MergeVariables(varMaps...)
}

// RunDirective is responsible for actually executing a directive, using the `shell`
// package.
func (mgr *Manager) RunDirective(ctx context.Context, ds Directive) error {
	runID := ctx.Value(contextKeyRunID).(ulid.ULID)
	applyStart := time.Now()
	labels := prometheus.Labels{
		"directive": ds.String(),
	}
	metricManagerDirectiveRunTimestamp.With(labels).Set(float64(applyStart.Unix()))

	hostVarsMap := shell.MakeVariableMap(mgr.hostVariables)
	allTemplateData := mgr.getTemplateData(ctx, ds.String(), hostVarsMap, nil, hostVarsMap)

	renderedScript, err := templateScript(ctx, ds.String(), allTemplateData, mgr.funcMap)
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

// RunAll runs all of the Directives being managed by the Manager, followed by
// all of the Modules being managed by the Manager.
func (mgr *Manager) RunAll(ctx context.Context) {
	go func() {
		logger := mgr.logger.WithFields(log.Fields{
			"run_id": ctx.Value(contextKeyRunID).(ulid.ULID).String(),
		})
		metricManagerRunInProgress.With(prometheus.Labels{"manager": mgr.String()}).Set(1)

		defer func() {
			metricManagerRunInProgress.With(prometheus.Labels{"manager": mgr.String()}).Set(0)
			logger.Info("Finished run")
		}()

		if !mgr.runLock.TryLock() {
			logger.Warn("Manager run already in progress, aborting")
			return
		}
		defer mgr.runLock.Unlock()

		mgr.RunDirectives(ctx)
		mgr.RunModules(ctx)
	}()
}
