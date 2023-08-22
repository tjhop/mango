package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"

	"github.com/tjhop/mango/internal/inventory"
	"github.com/tjhop/mango/internal/shell"
)

// Directive is a wrapper struct that encapsulates an inventory.Directive
type Directive struct {
	d inventory.Directive
}

func (dir Directive) String() string { return dir.d.String() }

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
