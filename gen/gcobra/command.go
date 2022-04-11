package gcobra

import (
	"fmt"
	"os"
	"reflect"

	"github.com/spf13/cobra"

	"github.com/octago/sflags"
	"github.com/octago/sflags/internal/scan"
	"github.com/octago/sflags/internal/tag"
)

// Parse returns a root cobra Command to be used directly as an entry-point.
// The data interface parameter can be nil, or arbitrarily:
// - A simple group of options to bind at the local, root level
// - A struct containing substructs for postional parameters, and other with options.
func Parse(data interface{}) *cobra.Command {
	// The command is empty, so that the returned command can be
	// directly ran as a root application command, with calls like
	// cmd.Execute(), or cobra.CheckErr(cmd.Execute())
	cmd := &cobra.Command{
		Use:         os.Args[0], // By default, the command is the name of the binary. TODO: change this
		Annotations: map[string]string{},
	}

	// A command always accepts embedded
	// subcommand struct fields, so scan them.
	scanner := scanCommand(cmd, nil)

	// Scan the struct recursively, for both
	// arg/option groups and subcommands
	if err := scan.Type(data, scanner); err != nil {
		fmt.Println(err)
		fmt.Println("nil")
		return nil
	}

	// NOTE: should handle remote exec here

	// Sane defaults for working both in CLI and in closed-loop applications.
	cmd.TraverseChildren = true

	// Subcommands optional or not
	if !cmd.HasSubCommands() {
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			return nil
		}
	} else if _, isCmd, impl := sflags.IsCommand(reflect.ValueOf(data)); isCmd {
		setRuns(cmd, impl)
	}

	return cmd
}

// scan is in charge of building a recursive scanner, working on a
// given struct field at a time, checking for arguments, subcommands and option groups.
func scanCommand(cmd *cobra.Command, group *cobra.Group) scan.Handler {
	handler := func(val reflect.Value, sfield *reflect.StructField) (bool, error) {
		// Parse the tag or die tryin. We should find one, or we're not interested.
		mtag, none, err := tag.GetFieldTag(*sfield)
		if none || err != nil {
			return true, err
		}

		// If the field is marked as -one or more- positional arguments, we
		// return either on a successful scan of them, or with an error doing so.
		if found, err := positionals(cmd, mtag, val); found || err != nil {
			return found, err
		}

		// Else, if the field is marked as a subcommand, we either return on
		// a successful scan of the subcommand, or with an error doing so.
		if found, err := command(cmd, group, mtag, val); found || err != nil {
			return found, err
		}

		// Else, if the field is a struct group of options
		if found, err := flagsGroup(cmd, val, sfield); found || err != nil {
			return found, err
		}

		// Else, try scanning the field as a simple option flag
		return flagScan(cmd)(val, sfield)
	}

	return handler
}

// command finds if a field is marked as a subcommand, and if yes, scans it.
func command(cmd *cobra.Command, grp *cobra.Group, tag tag.MultiTag, val reflect.Value) (bool, error) {
	// Parse the command name on struct tag...
	name, _ := tag.Get("command")
	if len(name) == 0 {
		return false, nil
	}

	// ... and check the field implements at least the Commander interface
	val, implements, cmdType := sflags.IsCommand(val)
	if !implements && len(name) != 0 && cmdType == nil {
		return false, ErrNotCommander
	} else if !implements && len(name) == 0 {
		return false, nil // Skip to next field
	}

	// Always populate the maximum amount of information
	// in the new subcommand, so that when it scans recursively,
	// we can have a more granular context.
	subc := newCommand(name, tag, grp)

	// Bind the various pre/run/post implementations of our command.
	setRuns(subc, cmdType)

	// Scan the struct recursively, for both arg/option groups and subcommands
	scanner := scanCommand(subc, grp)
	if err := scan.Type(val.Interface(), scanner); err != nil {
		return true, err
	}

	// If we have more than one subcommands and that we are NOT
	// marked has having optional subcommands, remove our run function
	// function, so that help printing can behave accordingly.
	if _, isSet := tag.Get("subcommands-optional"); !isSet {
		if len(subc.Commands()) > 0 {
			cmd.RunE = nil
		}
	}

	// And bind this subcommand back to us
	cmd.AddCommand(subc)

	return true, nil
}

// builds a quick command template based on what has been specified through tags, and in context.
func newCommand(name string, mtag tag.MultiTag, parent *cobra.Group) *cobra.Command {
	subc := &cobra.Command{
		Use:         name,
		Annotations: map[string]string{},
	}

	subc.Short, _ = mtag.Get("description")
	subc.Long, _ = mtag.Get("long-description")
	subc.Aliases = mtag.GetMany("alias")
	_, subc.Hidden = mtag.Get("hidden")

	// Grouping the command ----------

	// - Either inherited from the group within which we are parsed.
	if parent != nil && parent.Group != "" {
		subc.Group = parent.Group
	}

	// - Either specifically mentionned on the command (thus has priority)
	if group, isSet := mtag.Get("group"); isSet {
		subc.Group = group
	}

	// TODO: here inherit from struct marked group, with commands and options.

	// TODO: namespace tags on commands ?

	return subc
}

// setRuns binds the various pre/run/post implementations to a cobra command.
func setRuns(cmd *cobra.Command, impl sflags.Commander) {
	// No implementation means that this command
	// requires subcommands by default.
	if impl == nil {
		return
	}

	// Main run
	cmd.RunE = func(c *cobra.Command, args []string) error {
		retargs := getRemainingArgs(c)
		cmd.SetArgs(retargs)

		return impl.Execute(retargs)
	}
}
