package manager

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"

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
	file, err := os.Stat(ds.String())
	if err != nil {
		return fmt.Errorf("Failed to stat directive script %s: %s", ds.String(), err)
	}

	// only run directive if modified within last 24h
	if file.ModTime().After(time.Now().Add(-(time.Hour * 24))) {
		ctx, runID := getOrSetRunID(ctx)
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
	}

	return nil
}

// RunDirectives runs all of the directive scripts being managed by the Manager
func (mgr *Manager) RunDirectives(ctx context.Context, logger *slog.Logger) {
	ctx, _ = getOrSetRunID(ctx)

	if len(mgr.directives) <= 0 {
		logger.InfoContext(ctx, "No Directives to run")
		return
	}

	logger.InfoContext(ctx, "Directive run started")
	defer logger.InfoContext(ctx, "Directive run finished")
	for _, d := range mgr.directives {
		dLogger := logger.With(
			slog.Group(
				"directive",
				slog.String("id", d.String()),
			),
		)

		dLogger.InfoContext(ctx, "Directive started")
		defer dLogger.InfoContext(ctx, "Directive finished")

		if err := mgr.RunDirective(ctx, d); err != nil {
			dLogger.LogAttrs(
				ctx,
				slog.LevelError,
				"Directive failed",
				slog.String("err", err.Error()),
			)
		}
	}
}
