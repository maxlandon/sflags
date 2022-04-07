package sflags

import (
	"reflect"

	"github.com/octago/sflags/internal/tag"
)

// Commander is the simplest and smallest interface that a type must
// implement to be a valid, local, client command. This command can
// be used either in a single-run CLI app, or in a closed-loop shell.
type Commander interface {
	// Execute runs the command implementation.
	// The args parameter is any argument that has not been parsed
	// neither on any parent command and/or its options, or this
	// command and/or its args/options.
	Execute(args []string) (err error)
}

// checks both tags and implementations.
func IsCommand(mtag tag.MultiTag, val reflect.Value) (bool, string, Commander) {
	name, _ := mtag.Get("command")
	if len(name) == 0 {
		return false, "", nil
	}

	// Initialize if needed
	var ptrval reflect.Value

	if val.Kind() == reflect.Ptr {
		ptrval = val

		if ptrval.IsNil() {
			ptrval.Set(reflect.New(ptrval.Type().Elem()))
		}
	} else {
		ptrval = val.Addr()
	}

	// Assert implementation
	cmd, implements := ptrval.Interface().(Commander)
	if !implements {
		return false, "", nil
	}

	return true, name, cmd
}
