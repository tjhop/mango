package main

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	dirCmd = &cobra.Command{
		Use:     "directive",
		Aliases: []string{"directives"},
		Short:   "Command to interact with mango directives in the inventory",
		Long:    "Command to interact with mango directive, including adding, deleting, etc",
	}

	dirAddCmd = &cobra.Command{
		Use:     "add",
		Aliases: addCmdAliases,
		Short:   "Create an empty directive with the provided name",
		Long: "Command to add a new directive by adding a new directory with the given" +
			" name and creating empty directive files to bootstrap",
		Args: cobra.ExactArgs(1),
		Run:  directiveAdd,
	}

	dirDeleteCmd = &cobra.Command{
		Use:     "delete",
		Aliases: delCmdAliases,
		Short:   "Delete the directive with the provided name",
		Long:    "Command to delete a directive by recursively removing it from the inventory",
		Args:    cobra.ExactArgs(1),
		Run:     directiveDelete,
	}

	dirListCmd = &cobra.Command{
		Use:     "list",
		Aliases: listCmdAliases,
		Short:   "List directives in the inventory",
		Long:    "List directives in the inventory",
		Args:    cobra.ExactArgs(0),
		Run:     directiveList,
	}
)

func init() {
	inventoryCmd.AddCommand(dirCmd)
	dirCmd.AddCommand(dirAddCmd)
	dirCmd.AddCommand(dirDeleteCmd)
	dirCmd.AddCommand(dirListCmd)
}

func directiveAdd(cmd *cobra.Command, args []string) {
	dirName := args[0]
	logger := slog.Default().With("component", "directive", "directive", dirName)

	dirDir := filepath.Join(viper.GetString("inventory.path"), "directives")
	dirPath := filepath.Join(dirDir, dirName)

	if err := inventoryAddFile(dirPath); err != nil {
		logger.Warn("Error creating directive file", "err", err, "file", dirPath)
	} else {
		logger.Debug("Created directive file", "file", dirPath)
	}
}

func directiveDelete(cmd *cobra.Command, args []string) {
	dirName := args[0]
	logger := slog.Default().With("component", "directive", "directive", dirName)

	dirDir := filepath.Join(viper.GetString("inventory.path"), "directives")
	dirPath := filepath.Join(dirDir, dirName)

	if err := inventoryRemoveAll(dirPath); err != nil {
		logger.Warn("Error deleting directive", "err", err)
	}
}

func directiveList(cmd *cobra.Command, args []string) {
	inv := loadInventory()
	for _, d := range inv.GetDirectives() {
		fmt.Println(d.String())
	}
}
