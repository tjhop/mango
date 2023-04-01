package inventory

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/tjhop/mango/internal/utils"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
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

// ParseDirectives looks for scripts in the inventory's `directives/` folder, and
// adds them to the inventory if they are executable. It also records the last modified time
// (mtime) of the file, so that the we can determine if it's been modified within the last 24h
// when we attempt to actually apply the directive scripts.
func (i *Inventory) ParseDirectives(ctx context.Context) error {
	commonLabels := prometheus.Labels{
		"inventory": i.inventoryPath,
		"component": "directives",
	}

	path := filepath.Join(i.inventoryPath, "directives")
	files, err := utils.GetFilesInDirectory(path)
	if err != nil {
		log.WithFields(log.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to parse directives")

		// inventory counts haven't been altered, no need to update here
		metricInventoryReloadFailedTotal.With(commonLabels).Inc()

		return err
	}

	var dirScripts []Directive

	for _, file := range files {
		if !file.IsDir() && !strings.HasPrefix(file.Name(), ".") {
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
