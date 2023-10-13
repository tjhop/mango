package inventory

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/tjhop/mango/pkg/utils"

	"github.com/prometheus/client_golang/prometheus"
)

// Directive contains fields that are relevant specifically to directive scripts, which
// are only executed if their modification time is within the last 24h
// Directive contains fields that represent a script in the inventory's directives directory.
// These scripts are executed first when changes are detected in the inventory, if and only if the
// script has a modification time within the last 24h.
// - ID: string idenitfying the directive script (generally the file path to the script)
type Directive struct {
	ID string
}

// String is a stringer to return the module ID
func (d Directive) String() string { return d.ID }

// ParseDirectives looks for scripts in the inventory's `directives/` folder and adds them
func (i *Inventory) ParseDirectives(ctx context.Context, logger *slog.Logger) error {
	commonLabels := prometheus.Labels{
		"inventory": i.inventoryPath,
		"component": "directives",
	}
	logger = logger.With(
		slog.Group(
			"inventory",
			slog.String("component", "directives"),
		),
	)

	path := filepath.Join(i.inventoryPath, "directives")
	files, err := utils.GetFilesInDirectory(path)
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to parse directives",
			slog.String("err", err.Error()),
			slog.String("path", path),
		)

		// inventory counts haven't been altered, no need to update here
		metricInventoryReloadFailedTotal.With(commonLabels).Inc()

		return err
	}

	var dirScripts []Directive

	for _, file := range files {
		if !file.IsDir() && !utils.IsHidden(file.Name()) {
			scriptPath := filepath.Join(path, file.Name())

			dirScripts = append(dirScripts, Directive{
				ID: scriptPath,
			})
		}
	}

	i.directives = dirScripts
	metricInventory.With(commonLabels).Set(float64(len(i.directives)))
	// directives are applicable to **all** systems, not just enrolled systems
	metricInventoryApplicable.With(commonLabels).Set(float64(len(i.directives)))
	metricInventoryReloadSeconds.With(commonLabels).Set(float64(time.Now().Unix()))
	metricInventoryReloadTotal.With(commonLabels).Inc()

	return nil
}
