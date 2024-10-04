package main

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	roleFiles = []string{"modules", "variables"}
	roleDirs  = []string{"templates"}

	roleCmd = &cobra.Command{
		Use:     "role",
		Aliases: []string{"roles"},
		Short:   "Command to interact with mango roles in the inventory",
		Long:    "Command to interact with mango role, including adding, deleting, etc",
	}

	roleAddCmd = &cobra.Command{
		Use:     "add",
		Aliases: addCmdAliases,
		Short:   "Create an empty role with the provided name",
		Long: "Command to add a new role by adding a new directory with the given" +
			" name and creating empty role files to bootstrap",
		Args: cobra.ExactArgs(1),
		Run:  roleAdd,
	}

	roleDeleteCmd = &cobra.Command{
		Use:     "delete",
		Aliases: delCmdAliases,
		Short:   "Delete the role with the provided name",
		Long:    "Command to delete a role by recursively removing it from the inventory",
		Args:    cobra.ExactArgs(1),
		Run:     roleDelete,
	}

	roleListCmd = &cobra.Command{
		Use:     "list",
		Aliases: listCmdAliases,
		Short:   "List roles in the inventory",
		Long:    "List roles in the inventory",
		Args:    cobra.ExactArgs(0),
		Run:     roleList,
	}
)

func init() {
	inventoryCmd.AddCommand(roleCmd)
	roleCmd.AddCommand(roleAddCmd)
	roleCmd.AddCommand(roleDeleteCmd)
	roleCmd.AddCommand(roleListCmd)
}

func roleAdd(cmd *cobra.Command, args []string) {
	roleName := args[0]
	logger := slog.Default().With("component", "role", "role", roleName)

	roleDir := filepath.Join(viper.GetString("inventory.path"), "roles")
	rolePath := filepath.Join(roleDir, roleName)

	if err := inventoryAddDir(rolePath); err != nil {
		logger.Warn("Error initializing role", "err", err)
	}

	for _, rFile := range roleFiles {
		file := filepath.Join(rolePath, rFile)
		if err := inventoryAddFile(file); err != nil {
			logger.Warn("Error creating role file", "err", err, "file", file)
		} else {
			logger.Debug("Created role file", "file", file)
		}
	}

	for _, rDir := range roleDirs {
		dir := filepath.Join(rolePath, rDir)
		if err := inventoryAddDir(dir); err != nil {
			logger.Warn("Error initializing role", "err", err, "dir", dir)
		} else {
			logger.Debug("Created role directory", "dir", dir)
		}
	}
}

func roleDelete(cmd *cobra.Command, args []string) {
	roleName := args[0]
	logger := slog.Default().With("component", "role", "role", roleName)

	roleDir := filepath.Join(viper.GetString("inventory.path"), "roles")
	rolePath := filepath.Join(roleDir, roleName)

	if err := inventoryRemoveAll(rolePath); err != nil {
		logger.Warn("Error deleting role", "err", err)
	}
}

func roleList(cmd *cobra.Command, args []string) {
	inv := loadInventory()
	for _, g := range inv.GetRoles() {
		fmt.Println(g.String())
	}
}
