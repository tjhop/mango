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
// - ModTime: the modification time of the script
type DirectiveScript struct {
	id      string
	script  Script
	modTime time.Time
}

// String is a stringer to return the module ID
func (d DirectiveScript) String() string { return d.id }

// Run is a wrapper to run the DirectiveScript's script and return any
// potential errors. DirectiveScripts are only run if they have been updated
// within the last 24h (ie, their mod time is within the last 24h).
func (d DirectiveScript) Run(ctx context.Context) error {
	// TODO: fix this. currently, mtime is only checked when modules are
	// parsed. it should be moved to be checked during runtime.
	recentThreshold, _ := time.ParseDuration("24h")
	if float64(time.Now().Unix()-d.modTime.Unix()) < recentThreshold.Seconds() {
		log.WithFields(log.Fields{
			"directive": d,
		}).Info("Directive recently modified, running it now")

		return d.script.Run(ctx)
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
		metricInventoryReloadSeconds.With(prometheus.Labels{
			"inventory": commonLabels["inventory"],
			"component": commonLabels["component"],
			"hostname":  commonLabels["hostname"],
			"result":    "failure",
		}).Set(float64(time.Now().Unix()))
		metricInventoryReloadTotal.With(prometheus.Labels{
			"inventory": commonLabels["inventory"],
			"component": commonLabels["component"],
			"hostname":  commonLabels["hostname"],
			"result":    "failure",
		}).Inc()

		return err
	}

	var dirScripts []DirectiveScript

	for _, file := range files {
		if !file.IsDir() {
			scriptPath := filepath.Join(path, file.Name())
			info, err := file.Info()
			if err != nil {
				log.WithFields(log.Fields{
					"path":  scriptPath,
					"error": err,
				}).Error("Failed to get file info")

				// inventory counts haven't been altered, no need to update here
				metricInventoryReloadSeconds.With(prometheus.Labels{
					"inventory": commonLabels["inventory"],
					"component": commonLabels["component"],
					"hostname":  commonLabels["hostname"],
					"result":    "failure",
				}).Set(float64(time.Now().Unix()))
				metricInventoryReloadTotal.With(prometheus.Labels{
					"inventory": commonLabels["inventory"],
					"component": commonLabels["component"],
					"hostname":  commonLabels["hostname"],
					"result":    "failure",
				}).Inc()

				return err
			}

			dirScripts = append(dirScripts, DirectiveScript{
				script: Script{
					ID:   file.Name(),
					Path: scriptPath,
				},
				modTime: info.ModTime(),
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
		"result":    "success",
	}).Set(float64(time.Now().Unix()))
	metricInventoryReloadTotal.With(prometheus.Labels{
		"inventory": commonLabels["inventory"],
		"component": commonLabels["component"],
		"hostname":  commonLabels["hostname"],
		"result":    "success",
	}).Inc()

	return nil
}
