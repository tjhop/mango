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

// Role contains fields that represent a single role in the inventory.
// - ID: string idenitfying the role (generally the file path to the role)
// - Modules: a []string of module names that satisfy this role
type Role struct {
	id      string
	modules []string
}

// String is a stringer to return the role ID
func (r Role) String() string { return r.id }

// ParseRoles searches for directories in the provided path. Each directory is
// treated as a role -- each role is checked for the appropriate `modules` file to
// parse for the list of modules for the role.
func (i *Inventory) ParseRoles(ctx context.Context, logger *slog.Logger) error {
	commonLabels := prometheus.Labels{
		"inventory": i.inventoryPath,
		"component": "roles",
	}
	logger = logger.With(
		slog.Group(
			"inventory",
			slog.String("component", "roles"),
		),
	)

	path := filepath.Join(i.inventoryPath, "roles")
	roleDirs, err := utils.GetFilesInDirectory(path)
	if err != nil {
		logger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to parse roles",
			slog.String("err", err.Error()),
			slog.String("path", path),
		)

		// inventory counts haven't been altered, no need to update here
		metricInventoryReloadFailedTotal.With(commonLabels).Inc()

		return err
	}

	var roles []Role

	for _, roleDir := range roleDirs {
		if roleDir.IsDir() {
			rolePath := filepath.Join(path, roleDir.Name())
			roleFiles, err := utils.GetFilesInDirectory(rolePath)
			if err != nil {
				logger.LogAttrs(
					ctx,
					slog.LevelError,
					"Failed to parse role files",
					slog.String("err", err.Error()),
					slog.String("path", rolePath),
				)

				// inventory counts haven't been altered, no need to update here
				metricInventoryReloadFailedTotal.With(commonLabels).Inc()

				return err
			}

			role := Role{id: rolePath}

			for _, roleFile := range roleFiles {
				if !roleFile.IsDir() && !strings.HasPrefix(roleFile.Name(), ".") {
					fileName := roleFile.Name()
					switch fileName {
					case "modules":
						var mods []string
						modPath := filepath.Join(rolePath, "modules")
						lines := utils.ReadFileLines(modPath)

						for line := range lines {
							if line.Err != nil {
								logger.LogAttrs(
									ctx,
									slog.LevelError,
									"Failed to read modules in role",
									slog.String("err", line.Err.Error()),
									slog.String("path", modPath),
								)
								// inventory counts haven't been altered, no need to update here
								metricInventoryReloadFailedTotal.With(commonLabels).Inc()

							} else {
								mods = append(mods, line.Text)
							}
						}

						role.modules = mods

					default:
						logger.LogAttrs(
							ctx,
							slog.LevelWarn,
							"Skipping file while parsing inventory",
							slog.String("path", filepath.Join(rolePath, fileName)),
						)
					}
				}
			}

			roles = append(roles, role)
		}
	}

	i.roles = roles
	metricInventory.With(commonLabels).Set(float64(len(i.roles)))
	numMyRoles := 0
	if i.IsEnrolled() {
		numMyRoles = len(i.GetRolesForSelf())
	}
	metricInventoryApplicable.With(commonLabels).Set(float64(numMyRoles))
	metricInventoryReloadSeconds.With(commonLabels).Set(float64(time.Now().Unix()))
	metricInventoryReloadTotal.With(commonLabels).Inc()

	return nil
}
