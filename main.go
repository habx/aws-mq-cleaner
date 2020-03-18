package main

import (
	"github.com/habx/aws-mq-cleaner/commands"
	"github.com/habx/aws-mq-cleaner/flags"
)

var version string

func main() {
	flags.Version = version
	commands.RootCommand.Execute()
}
