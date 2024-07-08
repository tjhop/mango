package inventory

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/tjhop/mango/pkg/utils"

	"github.com/prometheus/client_golang/prometheus"
)

// Role contains fields that represent a single role in the inventory.
// - ID: string idenitfying the role (generally the file path to the role)
// - Modules: a []string of module names that satisfy this role
// - variables: path to the variables file for this role, if present
// - templateFiles: slice of paths of user defined template files
type Role struct {
	id            string
	modules       []string
	variables     string
	templateFiles []string
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
	iLogger := logger.With(
		slog.Group(
			"inventory",
			slog.String("component", "roles"),
		),
	)

	path := filepath.Join(i.inventoryPath, "roles")
	roleDirs, err := utils.GetFilesInDirectory(path)
	if err != nil {
		iLogger.LogAttrs(
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
		if roleDir.IsDir() && !utils.IsHidden(roleDir.Name()) {
			rolePath := filepath.Join(path, roleDir.Name())
			roleFiles, err := utils.GetFilesInDirectory(rolePath)
			if err != nil {
				iLogger.LogAttrs(
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
				if roleFile.IsDir() && roleFile.Name() == "templates" {
					templatedir := filepath.Join(rolePath, "templates")

					// From docs:
					// > Glob ignores file system errors such
					// > as I/O errors reading directories.
					// > The only possible returned error is
					// > ErrBadPattern, when pattern is
					// > malformed.
					// ...I'm making the pattern. I know it's not malformed.
					matchedTpls, _ := filepath.Glob(filepath.Join(templatedir, "*.tpl"))
					role.templateFiles = matchedTpls
				}

				if !roleFile.IsDir() && !utils.IsHidden(roleFile.Name()) {
					fileName := roleFile.Name()
					switch fileName {
					case "modules":
						var mods []string
						modPath := filepath.Join(rolePath, "modules")
						lines := utils.ReadFileLines(modPath)

						for line := range lines {
							if line.Err != nil {
								iLogger.LogAttrs(
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
					case "variables":
						role.variables = filepath.Join(rolePath, "variables")
					default:
						iLogger.LogAttrs(
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
