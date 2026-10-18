package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cerberauth/scimply/cmd/serve"
	"github.com/cerberauth/x/telemetryx"
)

// argvioAPIKey is scimply's public_ingest-scoped Argvio API key, selecting the
// "scimply" tenant its usage analytics land in. It only allows sending usage
// data, so it is safe to embed.
const argvioAPIKey = "argv_live_pub_5htgEGZlZ4AjHhFlED7XI8nW2ecD1hOb" //nolint:gosec // ingest-only key, not a secret

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"

	sqaOptOut bool
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
	}
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(serve.NewServeCmd(projectVersion, commit, date))

	rootCmd.PersistentFlags().BoolVarP(&sqaOptOut, "sqa-opt-out", "", false, "Opt out of sending anonymous usage statistics and crash reports to help improve the tool")

	return rootCmd
}

func Execute(projectVersion, commit, date string) {
	c := NewRootCmd(projectVersion, commit, date)
	argvioShutdown := telemetryx.Instrument(c, projectVersion, argvioAPIKey)
	defer func() { _ = argvioShutdown(context.Background()) }()

	if err := c.Execute(); err != nil {
		_ = argvioShutdown(context.Background())

		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		// nolint: gocritic // false positive
		os.Exit(1)
	}
}

func main() {
	Execute(version, commit, date)
}
