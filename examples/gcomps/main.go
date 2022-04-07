package main

import (
	"github.com/rsteube/carapace"
	"github.com/spf13/cobra"

	"github.com/octago/sflags/gen/gcobra"
	"github.com/octago/sflags/gen/gcomp"
)

func main() {
	// Root command: os.Args[0]
	root := &cobra.Command{
		Use: "gcomps",
	}

	// Subcommands
	data := &Command{}
	listCmd := gcobra.Command("list",
		"A local command demonstrating a few reflags features",
		"A longer help string used in detail help/usage output",
		data,
	)
	// Details
	listCmd.SilenceUsage = true

	// Completions
	gcomp.Gen(listCmd, data, nil)

	// Bind
	root.AddCommand(listCmd)

	// Generate completions for root
	carapace.Gen(root)

	root.Execute()
}
