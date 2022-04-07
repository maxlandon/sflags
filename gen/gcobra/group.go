package gcobra

import (
	"reflect"

	"github.com/spf13/cobra"

	"github.com/octago/sflags"
	"github.com/octago/sflags/gen/gpflag"
	"github.com/octago/sflags/internal/scan"
	"github.com/octago/sflags/internal/tag"
)

// flagsGroup finds if a field is marked as a subgroup of options, and if yes, scans it recursively.
func flagsGroup(cmd *cobra.Command, val reflect.Value, sfield *reflect.StructField) (bool, error) {
	mtag, none, err := tag.GetFieldTag(*sfield)
	if none || err != nil {
		return true, err
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
	optionsGroup, isSet := mtag.Get("group")
	if isSet && optionsGroup != "" {
		cmd.AddGroup(&cobra.Group{
			Group: optionsGroup,
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

	return true, nil
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
