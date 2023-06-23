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

// Module contains fields that represent a single module in the inventory.
// - ID: string idenitfying the module (generally the file path to the module)
// - Apply: path to apply script for the module
// - Variables: path to variables file for the module, if present
// - Requires: path to requirements file for the module, if present
// - Test: path to test script to check module's application status
type Module struct {
	ID        string
	Apply     string
	Variables string
	Test      string
	Requires  string
}

// String is a stringer to return the module ID
func (m Module) String() string { return m.ID }

// ParseModules looks for modules in the inventory's `modules/` folder. It looks for
// folders within this directory, and then parses each directory into a Module struct.
// Each module folder is expected to contain files for `apply`, `variables`, and `test`,
// which get set to the corresponding fields in the Module struct for the module.
func (i *Inventory) ParseModules(ctx context.Context) error {
	commonLabels := prometheus.Labels{
		"inventory": i.inventoryPath,
		"component": "modules",
	}

	path := filepath.Join(i.inventoryPath, "modules")
	modDirs, err := utils.GetFilesInDirectory(path)
	if err != nil {
		log.WithFields(log.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to get files in directory")

		// inventory counts haven't been altered, no need to update here
		metricInventoryReloadFailedTotal.With(commonLabels).Inc()

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
				metricInventoryReloadFailedTotal.With(commonLabels).Inc()

				return err
			}

			mod := Module{ID: modPath}

			for _, modFile := range modFiles {
				if !modFile.IsDir() && !strings.HasPrefix(modFile.Name(), ".") {
					fileName := modFile.Name()
					switch fileName {
					case "apply":
						mod.Apply = filepath.Join(mod.ID, "apply")
					case "test":
						mod.Test = filepath.Join(mod.ID, "test")
					case "variables":
						mod.Variables = filepath.Join(mod.ID, "variables")
					case "requires":
						mod.Requires = filepath.Join(mod.ID, "requires")
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
	metricInventoryReloadSeconds.With(commonLabels).Set(float64(time.Now().Unix()))
	metricInventoryReloadTotal.With(commonLabels).Inc()

	return nil
}
