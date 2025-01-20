package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	defaultMangoAddr = "127.0.0.1:9555"

	mangoCmd = &cobra.Command{
		Use:   "mango",
		Short: "Command to interact with a running mango server",
		Long:  "Command to interact with a running mango server, including interacting with pprofs, metrics, etc",
	}
)

func init() {
	mangoCmdFlagSet := mangoCmd.PersistentFlags()
	mangoCmdFlagSet.String("address", defaultMangoAddr, "Address of the running mango server")
	if err := viper.BindPFlags(mangoCmdFlagSet); err != nil {
		panic(fmt.Errorf("Error binding flags for command <%s>: %w", "mango", err))
	}
	rootCmd.AddCommand(mangoCmd)
}
