package cmd

import (
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
	rootCmd.PersistentFlags().StringVarP(&helpers.LogLevel, "log-level", "l", "info", helpers.LogLevelMsg)
	rootCmd.PersistentFlags().BoolVarP(&helpers.ColoredOutput, "color", "c", true, helpers.LogLevelMsg)
	rootCmd.AddCommand(create.CreateCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
