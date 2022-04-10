package main

import (
	"fmt"

	"github.com/octago/sflags/gen/gcobra"
	"github.com/octago/sflags/gen/gcomp"
)

func main() {
	//
	// Root ----------------------------------------------------------
	//

	rootData := &Command{}
	rootCmd := gcobra.Parse(rootData)
	rootCmd.SilenceUsage = true
	rootCmd.Short = "A local command demonstrating a few reflags features"
	rootCmd.Long = "A longer help string used in detail help/usage output"

	// Completions (recursive)
	comps, _ := gcomp.Generate(rootCmd, rootData, nil)
	comps.Standalone()

	// Execute the command (application here)
	if err := rootCmd.Execute(); err != nil {
		return
	}

	// listCmd.Execute()
	fmt.Println("Target: " + fmt.Sprintf("%v", rootData.CompletedArguments.Target))
	fmt.Println("Other: " + fmt.Sprintf("%v", rootData.CompletedArguments.Other))
}
