package cmd

import (
	"github.com/spf13/cobra/doc"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// docsCmd represents the docs command
var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Generate the markdown documentation for kubectl-k6",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		dir := filepath.Join(".", "docs")
		err := os.MkdirAll(dir, os.ModePerm)
		cobra.CheckErr(err)
		if len(args) > 0 {
			dir = args[0]
		}
		err = doc.GenMarkdownTree(rootCmd, dir)
		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(docsCmd)
}
