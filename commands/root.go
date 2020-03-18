package commands

import (
	_sns "github.com/habx/aws-mq-cleaner/commands/sns"
	_sqs "github.com/habx/aws-mq-cleaner/commands/sqs"
	"github.com/habx/aws-mq-cleaner/logger"
	"go.uber.org/zap"

	"github.com/habx/aws-mq-cleaner/flags"
	"github.com/spf13/cobra"
)

// RootCommand is the main command of the CLI
var RootCommand = &cobra.Command{
	Use:               "aws-mq-cleaner",
	PersistentPreRunE: initConfig,
	Version:           flags.Version,
}
var l *zap.Logger

func init() {
	RootCommand.PersistentFlags().BoolVarP(&flags.Delete, "delete", "d", false, "Enable deletion")
	RootCommand.PersistentFlags().StringVarP(&flags.LogLevel, "loglevel", "v", "info", "Set level log [debug,info]")
	RootCommand.PersistentFlags().BoolVarP(&flags.NoHeader, "no-header", "", false, "Disable print tab header")
	RootCommand.PersistentFlags().StringVarP(&flags.ExcludePatten, "exclude-patten", "e", "", "Exclude patten regex (Example:dev-dlq)")

	// Subcommands
	RootCommand.AddCommand(_sns.Command)
	RootCommand.AddCommand(_sqs.Command)

	RootCommand.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version number and quit",
		Long:  `All software has versions. aws-mq-cleaner`,
		Run: func(cmd *cobra.Command, args []string) {
			l.Sugar().Infof(flags.Version)
		},
	})
}

func initConfig(cmd *cobra.Command, args []string) error {
	l = logger.GetLogger(flags.LogLevel)
	return nil
}
