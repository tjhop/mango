package inventory

import (
	"context"
	"log/slog"
	"path/filepath"
	"regexp"
	"time"

	"github.com/tjhop/mango/pkg/utils"

	glob_util "github.com/gobwas/glob"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ValidGroupFiles = []string{"glob", "regex", "roles", "modules", "variables"}
	ValidGroupDirs  = []string{"templates"}
)

// Group contains fields that represent a given group of hosts in the inventory.
// - id: string idenitfying the group
// - globs: a slice of glob patterns to match against the instance's hostname
// - patterns: a slice of regex patterns to match against the instance's hostname
// - roles: a slice of roles that are applied to this host
// - modules: a slice of ad-hoc module names applied to this host
// - variables: path to the variables file for this group, if present
// - templateFiles: slice of paths of user defined template files
type Group struct {
	id            string
	globs         []string
	patterns      []string
	modules       []string
	roles         []string
	variables     string
	templateFiles []string
}

// String is a stringer to return the group ID
func (g Group) String() string { return g.id }

// ParseGroups looks for groups in the inventory's `groups/` folder. It looks for
// folders within this directory, and then parses each directory into a Group struct.
// Each Group folder may contain a file `glob` containing a newline separated
// list of glob matches, and a `regex` file containing regular expression
// patterns for comparing groupnames.
func (i *Inventory) ParseGroups(ctx context.Context, logger *slog.Logger) error {
	commonLabels := prometheus.Labels{
		"inventory": i.inventoryPath,
		"component": "groups",
	}
	iLogger := logger.With(
		slog.Group(
			"inventory",
			slog.String("component", "groups"),
		),
	)

	path := filepath.Join(i.inventoryPath, "groups")
	groupDirs, err := utils.GetFilesInDirectory(path)
	if err != nil {
		iLogger.LogAttrs(
			ctx,
			slog.LevelError,
			"Failed to get files in directory",
			slog.String("err", err.Error()),
			slog.String("path", path),
		)

		// inventory counts haven't been altered, no need to update here
		metricInventoryReloadFailedTotal.With(commonLabels).Inc()

		return err
	}

	var groups []Group

	for _, groupDir := range groupDirs {
		if groupDir.IsDir() && !utils.IsHidden(groupDir.Name()) {
			groupPath := filepath.Join(path, groupDir.Name())
			groupFiles, err := utils.GetFilesInDirectory(groupPath)
			if err != nil {
				iLogger.LogAttrs(
					ctx,
					slog.LevelError,
					"Failed to parse group files",
					slog.String("err", err.Error()),
					slog.String("path", groupPath),
				)

				// inventory counts haven't been altered, no need to update here
				metricInventoryReloadFailedTotal.With(commonLabels).Inc()

				return err
			}

			group := Group{id: groupDir.Name()}

			for _, groupFile := range groupFiles {
				if groupFile.IsDir() && groupFile.Name() == "templates" {
					templatedir := filepath.Join(groupPath, "templates")

					// From docs:
					// > Glob ignores file system errors such
					// > as I/O errors reading directories.
					// > The only possible returned error is
					// > ErrBadPattern, when pattern is
					// > malformed.
					// ...I'm making the pattern. I know it's not malformed.
					matchedTpls, _ := filepath.Glob(filepath.Join(templatedir, "*.tpl"))
					group.templateFiles = matchedTpls
				}

				if !groupFile.IsDir() && !utils.IsHidden(groupFile.Name()) {
					fileName := groupFile.Name()
					switch fileName {
					case "glob":
						var globs []string
						globPath := filepath.Join(groupPath, "glob")
						lines := utils.ReadFileLines(globPath)

						for line := range lines {
							if line.Err != nil {
								iLogger.LogAttrs(
									ctx,
									slog.LevelError,
									"Failed to read globs for group",
									slog.String("err", line.Err.Error()),
									slog.String("path", globPath),
								)
							} else {
								globs = append(globs, line.Text)
							}
						}

						group.globs = globs
					case "regex":
						var patterns []string
						patternPath := filepath.Join(groupPath, "regex")
						lines := utils.ReadFileLines(patternPath)

						for line := range lines {
							if line.Err != nil {
								iLogger.LogAttrs(
									ctx,
									slog.LevelError,
									"Failed to read regexs for group",
									slog.String("err", line.Err.Error()),
									slog.String("path", patternPath),
								)
							} else {
								patterns = append(patterns, line.Text)
							}
						}

						group.patterns = patterns
					case "roles":
						var roles []string
						rolePath := filepath.Join(groupPath, "roles")
						lines := utils.ReadFileLines(rolePath)

						for line := range lines {
							if line.Err != nil {
								iLogger.LogAttrs(
									ctx,
									slog.LevelError,
									"Failed to read roles for group",
									slog.String("err", line.Err.Error()),
									slog.String("path", rolePath),
								)
							} else {
								roles = append(roles, line.Text)
							}
						}

						group.roles = roles
					case "modules":
						var mods []string
						modPath := filepath.Join(groupPath, "modules")
						lines := utils.ReadFileLines(modPath)

						for line := range lines {
							if line.Err != nil {
								iLogger.LogAttrs(
									ctx,
									slog.LevelError,
									"Failed to read modules for group",
									slog.String("err", line.Err.Error()),
									slog.String("path", modPath),
								)
							} else {
								mods = append(mods, line.Text)
							}
						}

						group.modules = mods
					case "variables":
						group.variables = filepath.Join(groupPath, "variables")
					default:
						iLogger.LogAttrs(
							ctx,
							slog.LevelWarn,
							"Skipping file while parsing inventory",
							slog.String("path", filepath.Join(groupPath, fileName)),
						)
					}
				}
			}

			groups = append(groups, group)
		}
	}

	i.groups = groups
	metricInventory.With(commonLabels).Set(float64(len(i.groups)))
	groupMatches := 0
	for _, group := range i.groups {
		if group.IsHostEnrolled(i.hostname) {
			groupMatches++
		}
	}
	metricInventoryApplicable.With(commonLabels).Set(float64(groupMatches))
	metricInventoryReloadSeconds.With(commonLabels).Set(float64(time.Now().Unix()))
	metricInventoryReloadTotal.With(commonLabels).Inc()

	return nil
}

func (g Group) MatchGlobs(hostname string) int {
	matched := 0

	for _, globPattern := range g.globs {
		glob, err := glob_util.Compile(globPattern)
		if err != nil {
			slog.LogAttrs(
				context.Background(),
				slog.LevelWarn,
				"Failed to compile glob pattern for matching",
				slog.String("err", err.Error()),
				slog.String("glob", globPattern),
			)
			continue
		}

		if glob.Match(hostname) {
			matched++
			continue
		}
	}

	return matched
}

func (g Group) MatchPatterns(hostname string) int {
	matched := 0

	for _, pattern := range g.patterns {
		validPattern, err := regexp.Compile(pattern)
		if err != nil {
			slog.LogAttrs(
				context.Background(),
				slog.LevelWarn,
				"Failed to compile regex pattern for matching",
				slog.String("err", err.Error()),
				slog.String("regex", pattern),
			)
			continue
		}

		if validPattern.MatchString(hostname) {
			matched++
			continue
		}
	}

	return matched
}

func (g Group) MatchAll(hostname string) int {
	return g.MatchGlobs(hostname) + g.MatchPatterns(hostname)
}

func (g Group) IsHostEnrolled(hostname string) bool {
	return g.MatchAll(hostname) > 0
}
