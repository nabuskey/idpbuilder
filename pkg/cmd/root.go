package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/cnoe-io/idpbuilder/pkg/cmd/create"
	"github.com/cnoe-io/idpbuilder/pkg/cmd/helpers"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "idpbuilder",
	Short: "Manage reference IDPs",
	Long:  "",
}

func init() {
	// expose zap flags directly to this cli
	zapfs := flag.NewFlagSet("zap", flag.ExitOnError)
	helpers.ZapOptions.BindFlags(zapfs)

	rootCmd.PersistentFlags().AddGoFlagSet(zapfs)
	rootCmd.AddCommand(create.CreateCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
