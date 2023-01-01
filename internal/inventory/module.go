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

// Module contains fields that represent a single module in the inventory.
// - ID: string idenitfying the module (generally the file path to the module)
// - Apply: path to apply script for the module
// - Variables: variables for the module, in key, value form
// - Test: path to test script to check module's application status
type Module struct {
	id        string
	apply     Script
	variables VariableMap
	test      Script
}

// String is a stringer to return the module ID
func (m Module) String() string { return m.id }

// Test is a wrapper to run the Module's `test` script and return any potential
// errors
func (m Module) Test(ctx context.Context) error {
	return m.test.Run(ctx)
}

// Apply is a wrapper to run the Module's `apply` script and return any
// potential errors
func (m Module) Apply(ctx context.Context) error {
	return m.apply.Run(ctx)
}

// Run is a wrapper to run the Module's `test` script, and if needed, run the
// Module's `apply` script to get the system to the desired state
func (m Module) Run(ctx context.Context) error {
	logger := log.WithFields(log.Fields{
		"module": m,
	})

	if err := m.Test(ctx); err != nil {
		logger.WithFields(log.Fields{
			"error": err,
		}).Warn("Module failed idempotency test, running apply script to get system in desired state")

		if err := m.Apply(ctx); err != nil {
			logger.WithFields(log.Fields{
				"error": err,
			}).Error("Module apply script failed, unable to get system to desired state")

			return err
		}

		logger.Info("Module apply script succeeded")
		return nil
	}

	logger.Info("Module passed idempotency test")
	return nil
}

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
				metricInventoryReloadTotal.With(prometheus.Labels{
					"inventory": commonLabels["inventory"],
					"component": commonLabels["component"],
					"hostname":  commonLabels["hostname"],
					"result":    "failure",
				}).Inc()

				return err
			}

			mod := Module{id: modPath}

			for _, modFile := range modFiles {
				if !modFile.IsDir() {
					fileName := modFile.Name()
					switch fileName {
					case "apply":
						mod.apply = Script{
							ID:   fileName,
							Path: filepath.Join(modPath, fileName),
						}
					case "test":
						mod.test = Script{
							ID:   fileName,
							Path: filepath.Join(modPath, fileName),
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

						mod.variables = vars
					default:
						log.WithFields(log.Fields{
							"file": fileName,
						}).Debug("Not sure what to do with this file, so skipping it.")
					}
				}
			}

			modules = append(modules, mod)
		}
	}

	i.modules = modules
	metricInventory.With(commonLabels).Set(float64(len(i.modules)))
	numMyMods := 0
	if i.IsEnrolled() {
		mods := i.GetModulesForSelf()
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
	}).Set(float64(time.Now().Unix()))
	metricInventoryReloadTotal.With(prometheus.Labels{
		"inventory": commonLabels["inventory"],
		"component": commonLabels["component"],
		"hostname":  commonLabels["hostname"],
		"result":    "success",
	}).Inc()

	return nil
}
