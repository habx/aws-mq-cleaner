package main

import (
	"log"

	"github.com/habx/aws-mq-cleaner/commands"
	"github.com/spf13/cobra/doc"
)

func main() {
	err := doc.GenMarkdownTree(commands.RootCommand, "./")
	if err != nil {
		log.Fatal(err)
	} else {
		log.Printf("doc successfully generated")
	}
}
