package inventory

import (
	"path/filepath"
	"time"

	"github.com/tjhop/mango/internal/self"
	"github.com/tjhop/mango/internal/utils"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

// Module contains fields that represent a single module in the inventory.
// - ID: string idenitfying the module (generally the file path to the module)
// - Apply: path to apply script for the module
// - Variables: variables for the module, in key, value form
// - Test: path to test script to check module's application status
type Module struct {
	ID        string
	Apply     Script
	Variables VariableMap
	Test      Script
}

// String is a stringer to return the module ID
func (m Module) String() string { return m.ID }

// ParseModules looks for modules in the inventory's `modules/` folder. It looks for
// folders within this directory, and then parses each directory into a Module struct.
// Each module folder is expected to contain files for `apply`, `variables`, and `test`,
// which get set to the corresponding fields in the Module struct for the module.
func (i *Inventory) ParseModules() error {
	commonLabels := prometheus.Labels{
		"inventory": i.inventoryPath,
		"component": "modules",
		"hostname":  self.GetHostname(),
	}

	path := filepath.Join(i.inventoryPath, "modules")
	modDirs, err := utils.GetFilesInDirectory(path)
	if err != nil {
		log.WithFields(log.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to get files in directory")

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

	var modules []Module

	for _, modDir := range modDirs {
		if modDir.IsDir() {
			modPath := filepath.Join(path, modDir.Name())
			modFiles, err := utils.GetFilesInDirectory(modPath)
			if err != nil {
				log.WithFields(log.Fields{
					"path":  modPath,
					"error": err,
				}).Error("Failed to parse module files")

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

			mod := Module{ID: modPath}

			for _, modFile := range modFiles {
				if !modFile.IsDir() {
					fileName := modFile.Name()
					switch fileName {
					case "apply":
						if utils.IsFileExecutableToAll(modFile) {
							mod.Apply = Script{
								ID:   fileName,
								Path: filepath.Join(modPath, fileName),
							}
						}
					case "test":
						if utils.IsFileExecutableToAll(modFile) {
							mod.Test = Script{
								ID:   fileName,
								Path: filepath.Join(modPath, fileName),
							}
						}
					case "variables":
						varsPath := filepath.Join(modPath, "variables")
						vars, err := ParseVariables(varsPath)
						if err != nil {
							log.WithFields(log.Fields{
								"file":  varsPath,
								"error": err,
							}).Error("Failed to parse variables for module")
						}

						mod.Variables = vars
					default:
						log.WithFields(log.Fields{
							"file": fileName,
						}).Debug("Not sure what to do with this file, so skipping it.")
					}

					modules = append(modules, mod)
				}
			}
		}
	}

	i.Modules = modules
	metricInventory.With(commonLabels).Set(float64(len(i.Modules)))
	numMyMods := 0
	if i.IsEnrolled() {
		mods, err := GetModulesForSelf()
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
			}).Error("Failed to get Modules for self")
		}
		numMyMods = len(mods)
	}
	metricInventoryApplicable.With(commonLabels).Set(float64(numMyMods))
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
