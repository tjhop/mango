package main

import (
	"fmt"
	"log/slog"
	"path"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	pprofProfiles = []string{"allocs", "block", "cmdline", "goroutine", "heap", "mutex", "profile", "threadcreate", "trace"}

	pprofCmd = &cobra.Command{
		Use:   "pprof",
		Short: "Command to simplify pprof interactions for mango",
		Long:  "Command to interact with pprof for mango, including collecting profiles and opening them via `pprof`",
	}

	pprofListProfilesCmd = &cobra.Command{
		Use:     "list-profiles",
		Aliases: []string{"list", "profiles", "all"},
		Short:   "Command to list all available pprof profiles from the given mango server",
		Long:    "Command to list all available pprof profiles from the given mango server",
		Args:    cobra.ExactArgs(0),
		Run:     pprofListProfiles,
	}

	pprofGetProfileCmd = &cobra.Command{
		Use:     "get",
		Aliases: []string{"collect"},
		Short:   "Command to get the specified profile from the given mango server",
		Long:    "Command to get the specified profile from the given mango server. Flags are available to help control pprof params for things like profile durations.",
		Args:    cobra.ExactArgs(1),
		Run:     pprofGetProfile,
	}
)

func init() {
	mangoCmd.AddCommand(pprofCmd)

	pprofCmd.AddCommand(pprofListProfilesCmd)

	pprofGetProfileCmdFlagSet := pprofGetProfileCmd.Flags()
	pprofGetProfileCmdFlagSet.Int("debug", 0, "Corresponds to debug query param - https://pkg.go.dev/net/http/pprof#hdr-Parameters")
	pprofGetProfileCmdFlagSet.Int("gc", 0, "Corresponds to gc query param - https://pkg.go.dev/net/http/pprof#hdr-Parameters")
	pprofGetProfileCmdFlagSet.Int("seconds", 0, "Corresponds to seconds query param - https://pkg.go.dev/net/http/pprof#hdr-Parameters")
	if err := viper.BindPFlags(pprofGetProfileCmdFlagSet); err != nil {
		panic(fmt.Errorf("Error binding flags for command <%s>: %w", "pprof get", err))
	}
	pprofCmd.AddCommand(pprofGetProfileCmd)
}

func pprofListProfiles(cmd *cobra.Command, args []string) {
	for _, p := range pprofProfiles {
		fmt.Println(p)
	}
}

func pprofGetProfile(cmd *cobra.Command, args []string) {
	addr := viper.GetString("address")
	profile := args[0]
	pprofProfilePath := path.Join("debug/pprof", profile)

	params := []urlParam{}
	debug := viper.GetInt("debug")
	if debug > 0 {
		params = append(params, urlParam{key: "debug", value: strconv.Itoa(debug)})
	}
	gc := viper.GetInt("gc")
	if gc > 0 {
		params = append(params, urlParam{key: "gc", value: strconv.Itoa(gc)})
	}
	seconds := viper.GetInt("seconds")
	if seconds > 0 {
		params = append(params, urlParam{key: "seconds", value: strconv.Itoa(seconds)})
	}

	body, err := httpGetBody(addr, pprofProfilePath, params)
	if err != nil {
		slog.Error("Error getting body for pprof profile", "err", err, "profile", profile)
	}

	fmt.Printf("%s", body)
}
