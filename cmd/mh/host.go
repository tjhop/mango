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
	hostCmd = &cobra.Command{
		Use:     "host",
		Aliases: []string{"hosts"},
		Short:   "Command to interact with mango hosts in the inventory",
		Long:    "Command to interact with mango host, including adding, deleting, etc",
	}

	hostAddCmd = &cobra.Command{
		Use:     "add",
		Aliases: addCmdAliases,
		Short:   "Create an empty host with the provided name",
		Long: "Command to add a new host by adding a new directory with the given" +
			" name and creating empty host files to bootstrap",
		Args: cobra.ExactArgs(1),
		Run:  hostAdd,
	}

	hostDeleteCmd = &cobra.Command{
		Use:     "delete",
		Aliases: delCmdAliases,
		Short:   "Delete the host with the provided name",
		Long:    "Command to delete a host by recursively removing it from the inventory",
		Args:    cobra.ExactArgs(1),
		Run:     hostDelete,
	}

	hostListCmd = &cobra.Command{
		Use:     "list",
		Aliases: listCmdAliases,
		Short:   "List hosts in the inventory",
		Long:    "List hosts in the inventory",
		Args:    cobra.ExactArgs(0),
		Run:     hostList,
	}
)

func init() {
	inventoryCmd.AddCommand(hostCmd)
	hostCmd.AddCommand(hostAddCmd)
	hostCmd.AddCommand(hostDeleteCmd)
	hostCmd.AddCommand(hostListCmd)
}

func hostAdd(cmd *cobra.Command, args []string) {
	hostName := args[0]
	logger := slog.Default().With("component", "host", "host", hostName)

	hostDir := filepath.Join(viper.GetString("inventory.path"), "hosts")
	hostPath := filepath.Join(hostDir, hostName)

	if err := inventoryAddDir(hostPath); err != nil {
		logger.Warn("Error initializing host", "err", err)
	}

	for _, hFile := range inventory.ValidHostFiles {
		file := filepath.Join(hostPath, hFile)
		if err := inventoryAddFile(file); err != nil {
			logger.Warn("Error creating host file", "err", err, "file", file)
		} else {
			logger.Debug("Created host file", "file", file)
		}
	}

	for _, hDir := range inventory.ValidHostDirs {
		dir := filepath.Join(hostPath, hDir)
		if err := inventoryAddDir(dir); err != nil {
			logger.Warn("Error initializing host", "err", err, "dir", dir)
		} else {
			logger.Debug("Created host directory", "dir", dir)
		}
	}
}

func hostDelete(cmd *cobra.Command, args []string) {
	hostName := args[0]
	logger := slog.Default().With("component", "host", "host", hostName)

	hostDir := filepath.Join(viper.GetString("inventory.path"), "hosts")
	hostPath := filepath.Join(hostDir, hostName)

	if err := inventoryRemoveAll(hostPath); err != nil {
		logger.Warn("Error deleting host", "err", err)
	}
}

func hostList(cmd *cobra.Command, args []string) {
	var hosts []inventory.Host
	inv := loadInventory()

	if viper.GetBool("enrolled-only") && inv.IsEnrolled() {
		host, _ := inv.GetHost(inv.GetHostname())
		hosts = append(hosts, host)
	} else {
		hosts = inv.GetHosts()
	}

	for _, h := range hosts {
		fmt.Println(h.String())
	}
}
