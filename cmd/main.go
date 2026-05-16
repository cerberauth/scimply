package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cerberauth/scimply/cmd/serve"
	"github.com/cerberauth/x/telemetryx"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"

	sqaOptOut    bool
	otelShutdown func(context.Context) error
)

var name = "scimply"

func NewRootCmd(projectVersion, commit, date string) (cmd *cobra.Command) {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version number of this application",
		Long:  `All software has versions. This is this application's`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(projectVersion + " (commit=" + commit + ", built=" + date + ")")
		},
	}

	rootCmd := &cobra.Command{
		Use:     name,
		Version: projectVersion + " (commit=" + commit + ", built=" + date + ")",
		Short:   "SCIM 2.0 server",
		Long:    "scimply is a production-ready SCIM 2.0 server.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if !sqaOptOut {
				otelShutdown, _ = telemetryx.New(cmd.Context(), name, projectVersion, telemetryx.WithCommit(commit), telemetryx.WithBuildDate(date))
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if otelShutdown != nil {
				_ = otelShutdown(cmd.Context())
				otelShutdown = nil
			}
		},
	}
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(serve.NewServeCmd())

	rootCmd.PersistentFlags().BoolVarP(&sqaOptOut, "sqa-opt-out", "", false, "Opt out of sending anonymous usage statistics and crash reports to help improve the tool")

	return rootCmd
}

func Execute(projectVersion, commit, date string) {
	c := NewRootCmd(projectVersion, commit, date)
	defer func() {
		if otelShutdown != nil {
			_ = otelShutdown(context.Background())
			otelShutdown = nil
		}
	}()

	if err := c.Execute(); err != nil {
		if otelShutdown != nil {
			_ = otelShutdown(context.Background())
			otelShutdown = nil
		}

		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		// nolint: gocritic // false positive
		os.Exit(1)
	}
}

func main() {
	Execute(version, commit, date)
}
