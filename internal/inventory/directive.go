package inventory

import (
	"context"
	"path/filepath"
	"time"

	"github.com/tjhop/mango/internal/self"
	"github.com/tjhop/mango/internal/utils"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// DirectiveScript contains fields that are relevant specifically to directive scripts, which
// are only executed if their modification time is within the last 24h
// DirectiveScript contains fields that represent a script in the inventory's directives directory.
// These scripts are executed first when changes are detected in the inventory, if and only if the
// script has a modification time within the last 24h.
// - ID: string idenitfying the directive script (generally the file path to the script)
// - Script: embedded `Script` object
type DirectiveScript struct {
	id      string
	script  Script
}

// String is a stringer to return the module ID
func (d DirectiveScript) String() string { return d.id }

// Run is a wrapper to run the DirectiveScript's script and return any
// potential errors. DirectiveScripts are only run if they have been updated
// within the last 24h (ie, their mod time is within the last 24h).
func (d DirectiveScript) Run(ctx context.Context) error {
	logger := log.WithFields(log.Fields{
		"directive": d,
	})

	threshold, _ := time.ParseDuration("24h")
	modTime := utils.GetFileModifiedTime(d.script.Path).Unix()
	recent := time.Now().Unix() - modTime
	if float64(recent) < threshold.Seconds() {
		logger.Info("Directive recently modified, running now")

		if err := d.script.Run(ctx); err != nil {
			logger.WithFields(log.Fields{
				"error": err,
			}).Error("Directive script failed, unable to get system to desired state")

			return err
		}

		logger.Info("Directive script succeeded")

		return nil
	}

	return nil
}

// ParseDirectives looks for scripts in the inventory's `directives/` folder, and
// adds them to the inventory if they are executable. It also records the last modified time
// (mtime) of the file, so that the we can determine if it's been modified within the last 24h
// when we attempt to actually apply the directive scripts.
func (i *Inventory) ParseDirectives() error {
	commonLabels := prometheus.Labels{
		"inventory": i.inventoryPath,
		"component": "directives",
		"hostname":  self.GetHostname(),
	}

	path := filepath.Join(i.inventoryPath, "directives")
	files, err := utils.GetFilesInDirectory(path)
	if err != nil {
		log.WithFields(log.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to parse directives")

		// inventory counts haven't been altered, no need to update here
		metricInventoryReloadFailedTotal.With(prometheus.Labels{
			"inventory": commonLabels["inventory"],
			"component": commonLabels["component"],
			"hostname":  commonLabels["hostname"],
		}).Inc()

		return err
	}

	var dirScripts []DirectiveScript

	for _, file := range files {
		if !file.IsDir() {
			scriptPath := filepath.Join(path, file.Name())

			dirScripts = append(dirScripts, DirectiveScript{
				script: Script{
					ID:   file.Name(),
					Path: scriptPath,
				},
			})
		}
	}

	i.directives = dirScripts
	metricInventory.With(commonLabels).Set(float64(len(i.directives)))
	// directives are applicable to **all** systems, not just enrolled systems
	metricInventoryApplicable.With(commonLabels).Set(float64(len(i.directives)))
	metricInventoryReloadSeconds.With(prometheus.Labels{
		"inventory": commonLabels["inventory"],
		"component": commonLabels["component"],
		"hostname":  commonLabels["hostname"],
	}).Set(float64(time.Now().Unix()))
	metricInventoryReloadTotal.With(prometheus.Labels{
		"inventory": commonLabels["inventory"],
		"component": commonLabels["component"],
		"hostname":  commonLabels["hostname"],
	}).Inc()

	return nil
}
