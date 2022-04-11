package gcobra

import (
	"reflect"

	"github.com/spf13/cobra"

	"github.com/octago/sflags"
	"github.com/octago/sflags/gen/gpflag"
	"github.com/octago/sflags/internal/scan"
	"github.com/octago/sflags/internal/tag"
)

// flagScan builds a small struct field handler so that we can scan
// it as an option and add it to our current command flags.
func flagScan(cmd *cobra.Command) scan.Handler {
	flagScanner := func(val reflect.Value, sfield *reflect.StructField) (bool, error) {
		// Parse a single field, returning one or more generic Flags
		flags, found := sflags.ParseField(val, *sfield)
		if !found {
			return false, nil
		}

		// Put these flags into the command's flagset.
		gpflag.GenerateTo(flags, cmd.Flags())

		return true, nil
	}

	return flagScanner
}

// flagsGroup finds if a field is marked as a subgroup of options, and if yes, scans it recursively.
func flagsGroup(cmd *cobra.Command, val reflect.Value, sfield *reflect.StructField) (bool, error) {
	mtag, skip, err := tag.GetFieldTag(*sfield)
	if err != nil {
		return true, err
	}
	if skip {
		return false, nil
	}

	description, _ := mtag.Get("description")

	var ptrval reflect.Value

	if val.Kind() == reflect.Ptr {
		ptrval = val

		if ptrval.IsNil() {
			ptrval.Set(reflect.New(ptrval.Type()))
		}
	} else {
		ptrval = val.Addr()
	}

	// We are either waiting for:
	// A group of options ("group" is the legacy name)
	var groupName string
	legacyGroup, legacyIsSet := mtag.Get("group")
	optionsGroup, optionsIsSet := mtag.Get("options")
	if legacyIsSet {
		groupName = legacyGroup
	} else if optionsIsSet {
		groupName = optionsGroup
	}
	if legacyIsSet && legacyGroup != "" {
		cmd.AddGroup(&cobra.Group{
			Group: groupName,
			Title: description,
		})

		err := addFlagSet(cmd, mtag, ptrval.Interface())

		return true, err
	}

	// Or a group of commands and options
	commandGroup, isSet := mtag.Get("commands")
	if isSet {
		var group *cobra.Group
		if !isStringFalsy(commandGroup) {
			group = &cobra.Group{
				Group: commandGroup,
				Title: description,
			}
			cmd.AddGroup(group)
		}

		// Parse for commands
		scannerCommand := scanCommand(cmd, group)
		err := scan.Type(ptrval.Interface(), scannerCommand)

		return true, err
	}

	// If we are here, we didn't find a command or a group.
	return false, nil
}

// addFlagSet scans a struct (potentially nested) for flag sets to bind to the command.
func addFlagSet(cmd *cobra.Command, mtag tag.MultiTag, data interface{}) error {
	var flagOpts []sflags.OptFunc

	// New change, in order to easily propagate parent namespaces
	// in heavily/specially nested option groups at bind time.
	delim, _ := mtag.Get("namespace-delimiter")

	namespace, _ := mtag.Get("namespace")
	if namespace != "" {
		flagOpts = append(flagOpts, sflags.Prefix(namespace+delim))
	}

	envNamespace, _ := mtag.Get("env-namespace")
	if envNamespace != "" {
		flagOpts = append(flagOpts, sflags.EnvPrefix(envNamespace))
	}

	// Create a new set of flags in which we will put our options
	flags, err := gpflag.Parse(data, flagOpts...)
	if err != nil {
		return err
	}

	// hidden, _ := mtag.Get("hidden")
	flags.SetInterspersed(true)

	persistent, _ := mtag.Get("persistent")
	if persistent != "" {
		cmd.PersistentFlags().AddFlagSet(flags)
	} else {
		cmd.Flags().AddFlagSet(flags)
	}

	return nil
}

func isStringFalsy(s string) bool {
	return s == "" || s == "false" || s == "no" || s == "0"
}
