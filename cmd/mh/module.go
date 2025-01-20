package main

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/tjhop/mango/internal/inventory"
)

var (
	moduleCmd = &cobra.Command{
		Use:     "module",
		Aliases: []string{"mod", "modules"},
		Short:   "Command to interact with mango modules in the inventory",
		Long:    "Command to interact with mango module, including adding, deleting, etc",
	}

	modAddCmd = &cobra.Command{
		Use:     "add",
		Aliases: addCmdAliases,
		Short:   "Create an empty module with the provided name",
		Long: "Command to add a new module by adding a new directory with the given" +
			" name and creating empty module files to bootstrap",
		Args: cobra.ExactArgs(1),
		Run:  moduleAdd,
	}

	modDeleteCmd = &cobra.Command{
		Use:     "delete",
		Aliases: delCmdAliases,
		Short:   "Delete the module with the provided name",
		Long:    "Command to delete a module by recursively removing it from the inventory",
		Args:    cobra.ExactArgs(1),
		Run:     moduleDelete,
	}

	modListCmd = &cobra.Command{
		Use:     "list",
		Aliases: listCmdAliases,
		Short:   "List modules in the inventory",
		Long:    "Command to list modules in the inventory",
		Args:    cobra.ExactArgs(0),
		Run:     moduleList,
	}
)

func init() {
	inventoryCmd.AddCommand(moduleCmd)
	moduleCmd.AddCommand(modAddCmd)
	moduleCmd.AddCommand(modDeleteCmd)

	modListCmdFlagSet := inventoryCmd.PersistentFlags()
	modListCmdFlagSet.Bool("enrolled-only", false, "Only return modules that the provided host is enrolled for")
	if err := viper.BindPFlags(modListCmdFlagSet); err != nil {
		panic(fmt.Errorf("Error binding flags for command <%s>: %w", "inventory", err))
	}
	moduleCmd.AddCommand(modListCmd)
}

func moduleAdd(cmd *cobra.Command, args []string) {
	modName := args[0]
	logger := slog.Default().With("component", "module", "module", modName)

	modDir := filepath.Join(viper.GetString("inventory.path"), "modules")
	modPath := filepath.Join(modDir, modName)

	if err := inventoryAddDir(modPath); err != nil {
		logger.Warn("Error initializing module", "err", err)
	}

	for _, mFile := range inventory.ValidModuleFiles {
		file := filepath.Join(modPath, mFile)
		if err := inventoryAddFile(file); err != nil {
			logger.Warn("Error creating module file", "err", err, "file", file)
		} else {
			logger.Debug("Created module file", "file", file)
		}
	}

	for _, mDir := range inventory.ValidModuleDirs {
		dir := filepath.Join(modPath, mDir)
		if err := inventoryAddDir(dir); err != nil {
			logger.Warn("Error initializing module", "err", err, "dir", dir)
		} else {
			logger.Debug("Created module directory", "dir", dir)
		}
	}
}

func moduleDelete(cmd *cobra.Command, args []string) {
	modName := args[0]
	logger := slog.Default().With("component", "module", "module", modName)

	modDir := filepath.Join(viper.GetString("inventory.path"), "modules")
	modPath := filepath.Join(modDir, modName)

	if err := inventoryRemoveAll(modPath); err != nil {
		logger.Warn("Error deleting module", "err", err)
	}
}

func moduleList(cmd *cobra.Command, args []string) {
	var modules []inventory.Module
	inv := loadInventory()

	if viper.GetBool("enrolled-only") && inv.IsEnrolled() {
		modules = inv.GetModulesForSelf()
	} else {
		modules = inv.GetModules()
	}

	for _, mod := range modules {
		fmt.Println(mod.String())
	}
}
