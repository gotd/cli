package main

import (
	"os"

	"github.com/go-faster/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// newDocsCmd generates documentation for the whole command tree. It is hidden
// because it is a maintenance/release tool, not an end-user command.
func newDocsCmd(root *cobra.Command) *cobra.Command {
	var (
		dir    string
		format string
	)

	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Generate command-tree documentation (Markdown or man pages)",
		Long: `Generate reference documentation for every command from the command tree,
so the published docs always match the code.`,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := os.MkdirAll(dir, 0o750); err != nil {
				return err
			}
			switch format {
			case "markdown", "md":
				return doc.GenMarkdownTree(root, dir)
			case "man":
				return doc.GenManTree(root, &doc.GenManHeader{Title: "TG", Section: "1"}, dir)
			default:
				return errors.Errorf("unknown format %q (want markdown or man)", format)
			}
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&dir, "dir", "docs", "output directory")
	fs.StringVar(&format, "format", "markdown", "output format: markdown or man")
	registerEnumCompletion(cmd, "format", []string{"markdown", "man"})

	return cmd
}
