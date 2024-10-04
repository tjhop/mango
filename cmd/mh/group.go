package main

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/tjhop/mango/internal/inventory"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	groupCmd = &cobra.Command{
		Use:     "group",
		Aliases: []string{"groups"},
		Short:   "Command to interact with mango groups in the inventory",
		Long:    "Command to interact with mango group, including adding, deleting, etc",
	}

	groupAddCmd = &cobra.Command{
		Use:     "add",
		Aliases: addCmdAliases,
		Short:   "Create an empty group with the provided name",
		Long: "Command to add a new group by adding a new directory with the given" +
			" name and creating empty group files to bootstrap",
		Args: cobra.ExactArgs(1),
		Run:  groupAdd,
	}

	groupDeleteCmd = &cobra.Command{
		Use:     "delete",
		Aliases: delCmdAliases,
		Short:   "Delete the group with the provided name",
		Long:    "Command to delete a group by recursively removing it from the inventory",
		Args:    cobra.ExactArgs(1),
		Run:     groupDelete,
	}

	groupListCmd = &cobra.Command{
		Use:     "list",
		Aliases: listCmdAliases,
		Short:   "List groups in the inventory",
		Long:    "List groups in the inventory",
		Args:    cobra.ExactArgs(0),
		Run:     groupList,
	}
)

func init() {
	inventoryCmd.AddCommand(groupCmd)
	groupCmd.AddCommand(groupAddCmd)
	groupCmd.AddCommand(groupDeleteCmd)
	groupCmd.AddCommand(groupListCmd)
}

func groupAdd(cmd *cobra.Command, args []string) {
	groupName := args[0]
	logger := slog.Default().With("component", "group", "group", groupName)

	groupDir := filepath.Join(viper.GetString("inventory.path"), "groups")
	groupPath := filepath.Join(groupDir, groupName)

	if err := inventoryAddDir(groupPath); err != nil {
		logger.Warn("Error initializing group", "err", err)
	}

	for _, gFile := range inventory.ValidGroupFiles {
		file := filepath.Join(groupPath, gFile)
		if err := inventoryAddFile(file); err != nil {
			logger.Warn("Error creating group file", "err", err, "file", file)
		} else {
			logger.Debug("Created group file", "file", file)
		}
	}

	for _, gDir := range inventory.ValidGroupDirs {
		dir := filepath.Join(groupPath, gDir)
		if err := inventoryAddDir(dir); err != nil {
			logger.Warn("Error initializing group", "err", err, "dir", dir)
		} else {
			logger.Debug("Created group directory", "dir", dir)
		}
	}
}

func groupDelete(cmd *cobra.Command, args []string) {
	groupName := args[0]
	logger := slog.Default().With("component", "group", "group", groupName)

	groupDir := filepath.Join(viper.GetString("inventory.path"), "groups")
	groupPath := filepath.Join(groupDir, groupName)

	if err := inventoryRemoveAll(groupPath); err != nil {
		logger.Warn("Error deleting group", "err", err)
	}
}

func groupList(cmd *cobra.Command, args []string) {
	var groups []inventory.Group
	inv := loadInventory()

	if viper.GetBool("enrolled-only") && inv.IsEnrolled() {
		groups = inv.GetGroupsForSelf()
	} else {
		groups = inv.GetGroups()
	}

	for _, g := range groups {
		fmt.Println(g.String())
	}
}
