package gcomp

import (
	"reflect"

	"github.com/octago/sflags/internal/tag"
	comp "github.com/rsteube/carapace"
)

// Completer represents a type that is able to return some
// completions based on the current carapace Context.
type Completer interface {
	Complete(ctx comp.Context) comp.Action
}

// the appropriate number of completers (equivalents carapace.ActionCallback)
// to be returned, for this field/requirements only.
func typeCompleter(val reflect.Value) comp.CompletionCallback {
	// Always check that the type itself does implement, even if
	// it's a list of type X that implements the completer as well.
	// If yes, we return this implementation, since it has priority.
	if val.Type().Kind() == reflect.Slice {
		i := val.Interface()
		if completer, ok := i.(Completer); ok {
			return completer.Complete
		}

		if val.CanAddr() {
			if completer, ok := val.Addr().Interface().(Completer); ok {
				return completer.Complete
			}
		}

		// Else we reassign the value to the list type.
		val = reflect.New(val.Type().Elem())
	}

	i := val.Interface()
	if completer, ok := i.(Completer); ok {
		return completer.Complete
	}

	if val.CanAddr() {
		if completer, ok := val.Addr().Interface().(Completer); ok {
			return completer.Complete
		}
	}

	return nil
}

// taggedCompletions builds a list of completion actions with struct tag specs.
func taggedCompletions(tag tag.MultiTag) (action comp.Action, found bool) {
	return
}
