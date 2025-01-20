package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/tjhop/mango/internal/inventory"
)

var (
	inventoryDirectories = []string{"groups", "hosts", "modules", "roles", "directives"}

	inventoryCmd = &cobra.Command{
		Use:     "inventory",
		Aliases: []string{"inv"},
		Short:   "Command to interact with mango inventory",
		Long:    "Command to interact with the mango inventory, such as initializing skeleton inventory directory structures",
	}

	invInitCmd = &cobra.Command{
		Use:     "init",
		Aliases: []string{"create", "new"},
		Short:   "Create an empty inventory",
		Long:    "Command to initialize a skeleton directory structure for use with mango inventory",
		Args:    cobra.ExactArgs(1),
		Run:     inventoryInit,
	}
)

func loadInventory() *inventory.Inventory {
	logger := slog.Default().With("component", "inventory")
	inventoryPath := viper.GetString("inventory.path")
	hostname := viper.GetString("hostname")

	inv := inventory.NewInventory(inventoryPath, hostname)
	logger.Debug("Created new inventory", "inventory_path", inventoryPath, "hostname", hostname)
	inv.Reload(context.Background(), logger)
	return inv
}

func init() {
	inventoryCmdFlagSet := inventoryCmd.PersistentFlags()
	inventoryCmdFlagSet.StringP("inventory.path", "i", "", "Path to mango configuration inventory")
	inventoryCmdFlagSet.String("hostname", "", "(Requires root) Custom hostname to use [default is system hostname]")
	if err := viper.BindPFlags(inventoryCmdFlagSet); err != nil {
		panic(fmt.Errorf("Error binding flags for command <%s>: %w", "inventory", err))
	}
	rootCmd.AddCommand(inventoryCmd)

	inventoryCmd.AddCommand(invInitCmd)
}

func inventoryInit(cmd *cobra.Command, args []string) {
	logger := slog.Default().With("component", "inventory")

	// attempt to make all directories and place a `.gitkeep` file inside them
	inventoryPath := viper.GetString("inventory.path")
	for _, inventoryDir := range inventoryDirectories {
		dir := filepath.Join(inventoryPath, inventoryDir)
		if err := inventoryAddDir(dir); err != nil {
			logger.Warn("Error initializing inventory", "err", err)
		} else {
			logger.Debug("Created inventory directory", "dir", dir)
		}
	}
}
