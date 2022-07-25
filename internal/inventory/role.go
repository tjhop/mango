package inventory

import (
	"path/filepath"

	"github.com/tjhop/mango/internal/utils"

	log "github.com/sirupsen/logrus"
)

// Role contains fields that represent a single role in the inventory.
// - ID: string idenitfying the role (generally the file path to the role)
// - Modules: a slice of module names that satisfy this role
type Role struct {
	ID      string
	Modules []string
}

// ParseRoles searches for directories in the provided path. Each directory is
// treated as a role -- each role is checked for the appropriate `modules` file to
// parse for the list of modules for the role.
func (i *Inventory) ParseRoles() error {
	path := filepath.Join(i.inventoryPath, "roles")
	roleDirs, err := utils.GetFilesInDirectory(path)
	if err != nil {
		log.WithFields(log.Fields{
			"path":  path,
			"error": err,
		}).Error("Failed to parse roles")

		return err
	}

	roles := make(map[string]Role)

	for _, roleDir := range roleDirs {
		if roleDir.IsDir() {
			rolePath := filepath.Join(path, roleDir.Name())
			roleFiles, err := utils.GetFilesInDirectory(rolePath)
			if err != nil {
				log.WithFields(log.Fields{
					"path":  rolePath,
					"error": err,
				}).Error("Failed to parse module files")

				return err
			}

			role := Role{ID: rolePath}

			for _, roleFile := range roleFiles {
				if !roleFile.IsDir() {
					fileName := roleFile.Name()
					switch fileName {
					case "modules":
						var mods []string
						modPath := filepath.Join(rolePath, "modules")
						lines := utils.ReadFileLines(modPath)

						for line := range lines {
							if line.Err != nil {
								log.WithFields(log.Fields{
									"path":  modPath,
									"error": line.Err,
								}).Error("Failed to read modules in role")
							} else {
								mods = append(mods, line.Text)
							}
						}

						role.Modules = mods

					default:
						log.WithFields(log.Fields{
							"file": fileName,
						}).Debug("Not sure what to do with this file, so skipping it.")
					}
				}
			}

			roles[role.ID] = role
		}
	}

	i.Roles = roles
	return nil
}
