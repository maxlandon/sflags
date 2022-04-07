package gcobra

import (
	"reflect"

	"github.com/spf13/cobra"

	"github.com/octago/sflags"
	"github.com/octago/sflags/internal/scan"
	"github.com/octago/sflags/internal/tag"
)

// Command generates a new cobra command from a struct, recursively
// scanning for options, subcommands, groups of either.
func Command(name, short, long string, data sflags.Commander) *cobra.Command {
	cmd := &cobra.Command{
		Use:         name,
		Short:       short,
		Long:        long,
		Annotations: map[string]string{},
	}

	// Bind the various pre/run/post implementations
	setRuns(cmd, data)

	// A command always accepts embedded
	// subcommand struct fields, so scan them.
	scanner := scanCommand(cmd, nil)

	// Scan the struct recursively, for both
	// arg/option groups and subcommands
	if err := scan.Type(data, scanner); err != nil {
		return nil
	}

	// NOTE: should handle remote exec here

	// Sane defaults for working both in CLI and in closed-loop applications.

	return cmd
}

// scan is in charge of building a recursive scanner, working on a
// given struct field at a time, checking for arguments, subcommands and option groups.
func scanCommand(cmd *cobra.Command, group *cobra.Group) scan.Handler {
	handler := func(val reflect.Value, sfield *reflect.StructField) (bool, error) {
		// Parse the tag or die tryin
		mtag, _, err := tag.GetFieldTag(*sfield)
		if err != nil {
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

		// Else, try scanning the field as a group of options
		return flagsGroup(cmd, val, sfield)
	}

	return handler
}

// command finds if a field is marked as a subcommand, and if yes, scans it.
func command(cmd *cobra.Command, grp *cobra.Group, tag tag.MultiTag, val reflect.Value) (bool, error) {
	// Parse the command name on struct tag, and check the field
	// implements at least the Commander interface
	isCmd, name, impl := sflags.IsCommand(tag, val)
	if !isCmd && len(name) != 0 && impl == nil {
		return false, ErrNotCommander
	} else if !isCmd && len(name) == 0 {
		return false, nil // Skip to next field
	}

	// Always populate the maximum amount of information
	// in the new subcommand, so that when it scans recursively,
	// we can have a more granular context.
	subc := newCommand(name, tag, grp)

	// A command always accepts embedded subcommand
	// struct fields, so scan them.
	scanner := scanCommand(subc, grp)

	// Bind the various pre/run/post implementations of our command.
	setRuns(subc, impl)

	// Scan the struct recursively, for both arg/option groups and subcommands
	if err := scan.Type(val.Addr().Interface(), scanner); err != nil {
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

		return impl.Execute(retargs)
	}
}
