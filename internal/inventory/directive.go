package inventory

import (
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
// - Script: embedded `Script` object
// - ModTime: the modification time of the script
type DirectiveScript struct {
	Script
	ModTime time.Time
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
		if !file.IsDir() && utils.IsFileExecutableToAll(file) {
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
				Script: Script{
					ID:   file.Name(),
					Path: scriptPath,
				},
				ModTime: info.ModTime(),
			})
		}
	}

	i.Directives = dirScripts
	metricInventoryTotal.With(commonLabels).Set(float64(len(i.Directives)))
	// directives are applicable to **all** systems, not just enrolled systems
	metricInventoryApplicableTotal.With(commonLabels).Set(float64(len(i.Directives)))
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

// GetDirectives returns a copy of the global inventory's slice of `DirectiveScript`'s
// Internally, it calls the `GetDirectives()` method against the global inventory.
// Should be used by other packages.
func GetDirectives() ([]DirectiveScript) {
	return globalInventory.Directives
}

// GetDirectives returns a copy of the inventory's slice of `DirectiveScript`'s
func (i *Inventory) GetDirectives() ([]DirectiveScript) {
	return i.Directives
}

// GetDirectivesForSelf returns a slice of `DirectiveScript`'s that are valid
// for the running system, based on it's hostname.
// All Directives are run against all enrolled systems, so if the system is 
// enrolled, this function will return all DirectiveScripts from the inventory.
func (i *Inventory) GetDirectivesForSelf() ([]DirectiveScript) {
	if IsEnrolled() {
		return i.GetDirectives()
	}

	return nil
}

// GetDirectivesForSelf returns a slice of `DirectiveScript`'s that are valid
// for the running system, based on it's hostname.
// All Directives are run against all enrolled systems, so if the system is 
// enrolled, this function will return all DirectiveScripts from the inventory.
func GetDirectivesForSelf() ([]DirectiveScript) {
	if IsEnrolled() {
		return GetDirectives()
	}

	return nil
}

