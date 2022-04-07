package gcobra

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"github.com/octago/sflags/internal/convert"
	"github.com/octago/sflags/internal/positional"
	"github.com/octago/sflags/internal/tag"
)

// positionals finds a struct tagged as containing positionals arguments and scans them.
func positionals(cmd *cobra.Command, stag tag.MultiTag, val reflect.Value) (bool, error) {
	// We need the struct to be marked as such
	if pargs, _ := stag.Get("positional-args"); len(pargs) == 0 {
		return false, nil
	}

	req, _ := stag.Get("required") // this is written on the struct, applies to all
	reqAll := len(req) != 0        // Each field will count as one required minimum
	doneReq := 0                   // Total count of required arguments

	// Scan all the fields on the struct and build the list of arguments
	// with their own requirements, and references to their values.
	positionals, reqTotal, err := positional.Scan(val, reqAll)
	if err != nil {
		return true, err
	}

	// The list of handlers that we will bind to our cobra Args function.
	handlers := make([]cobra.PositionalArgs, 0, len(positionals))

	// Each argument handler is being fed with some "real-time" values
	// and requirements, for more precise handling/converting, etc.
	for _, arg := range positionals {
		// Adjust the number of required arguments AFTER this one,
		// so that the argument can exit correctly if we have only
		// enough words to feed the remaining positional slots.
		reqTotal -= arg.Minimum

		// Make a custom handler for this positional to throw an error.
		errorHandler := positionalErrorHandler(positionals)

		// And build the handler (parser) function itself
		argumentHandler := positionalHandler(arg, &doneReq, reqTotal, errorHandler)
		handlers = append(handlers, argumentHandler)
	}

	// Finally, assemble all the parsers into our cobra Args function.
	cmd.Args = cobra.MatchAll(handlers...)

	return true, nil
}

// positionalHandler builds a function used to parse and validate
// none/one/more of the positional arguments passed by a cobra.Command.
func positionalHandler(arg *positional.Arg, start *int, needed int, err positionalErrHandler) cobra.PositionalArgs {
	handler := func(cmd *cobra.Command, args []string) error {
		// The stack of words that we will exhaust, and
		// the number of words we have parsed so far.
		stack := positional.MakeWords(args, start, needed)

		// Once we have consumed the words we wanted, we update the
		// command's return (non-consummed) arguments, to be passed
		// later to the Execute(args []string) implementation.
		defer setRemainingArgs(cmd, stack)

		// If the positional argument is not a list,
		// simply convert the individual value and return
		if !stack.Empty() && arg.Value.Type().Kind() != reflect.Slice {
			return convert.Value(stack.Pop(), arg.Value, arg.Tag)
		}

		// Else the argument accepts one or more items,
		// let it exhaust them smartly, breaking/returning
		// as soon as it has enough of them.
		return consumePositionals(stack, arg, err)
	}

	return handler
}

// consumePositionals parses one or more words from the current list of positionals into
// their struct fields, and returns once its own requirements are satisfied and/or the
// next positional arguments require words to be passed along.
func consumePositionals(stack *positional.Words, arg *positional.Arg, err positionalErrHandler) error {
	// As long as we've got a word, and nothing told us to quit.
	for !stack.Empty() {
		// If we have reached the maximum number of args we accept.
		if (arg.Minimum) > 0 && (arg.Maximum == stack.Parsed()) && arg.Maximum != -1 {
			return nil
		}

		// If we have the minimum required, but there are just
		// enough words for the next arguments, leave them the words.
		if (stack.Len() == stack.Required()) && stack.Parsed() >= arg.Minimum {
			return nil
		}

		// Else if we have not reached our maximum allowed number
		// of arguments, we are cleared to consume one.
		next := stack.Pop()

		// Any conversion error is fatal: TODO maybe handle errors
		if err := convert.Value(next, arg.Value, arg.Tag); err != nil {
			return err
		}
	}

	// If we are still lacking some required words,
	// but we have exhausted the available ones.
	if stack.Parsed() < arg.Minimum {
		return err(*arg, stack)
	}

	// Or we consumed all the arguments we wanted, without
	// error, so either exit because we are the last, or go
	// with the next argument handler we bound.
	return nil
}

func isRequired(p *positional.Arg) bool {
	return (p.Value.Type().Kind() != reflect.Slice && (p.Minimum > 0)) || // Both must be true
		p.Minimum != -1 || p.Maximum != -1 // And either of these
}

func setRemainingArgs(cmd *cobra.Command, stack *positional.Words) {
	if stack == nil || cmd == nil {
		return
	}

	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	// Add these arguments in an annotation to be used
	// in our Run implementation, where we pass just the
	// unparsed positional arguments to the command Execute(args []string).
	cmd.Annotations["sflags"] = strings.Join(stack.List(), " ")
}

func getRemainingArgs(cmd *cobra.Command) (args []string) {
	if cmd.Annotations == nil {
		return
	}

	if argString, found := cmd.Annotations["sflags"]; found {
		return strings.Split(argString, " ")
	}

	return
}

// positionalErrHandler is a function that builds a complete error message following
// the failure of an argument handler. We pass it some stuff allowing us to know
// where we're at, what we've parsed and what not.
type positionalErrHandler func(current positional.Arg, args *positional.Words) error

// positionalErrorHandler makes a handler to be used in our argument handlers when they fail.
func positionalErrorHandler(slots []*positional.Arg) positionalErrHandler {
	handler := func(arg positional.Arg, stack *positional.Words) (err error) {
		// Compute the list of arguments slots requiring some values
		if args := getRequiredArgNames(slots, arg); len(args) > 0 {
			var msg string

			if len(args) == 1 {
				msg = fmt.Sprintf("the required argument %s was not provided", args[0])
			} else {
				msg = fmt.Sprintf("the required arguments %s and %s were not provided",
					strings.Join(args[:len(args)-1], ", "), args[len(args)-1])
			}

			return newError(ErrRequired, msg)
		}

		return nil
	}

	return handler
}

// getRequiredArgNames is used by an argument handler to build the correct list of arguments we need.
func getRequiredArgNames(slots []*positional.Arg, current positional.Arg) (names []string) {
	// For each of the EXISTING positional argument fields
	for index, arg := range slots {
		// Ignore all positional arguments that have not
		// thrown an error: they have what they need.
		if index > 0 && index < current.Index {
			continue
		}

		// Non required positional won't appear in the message
		if !isRequired(arg) {
			continue
		}

		// If the positional is a single slot, we need its name
		if arg.Value.Type().Kind() != reflect.Slice {
			names = append(names, "`"+arg.Name+"`")

			continue
		}

		// If we have less words to parse than
		// the minimum required by this argument.
		if arg.Value.Len() < arg.Minimum {
			names = append(names, argHasNotEnough(*arg))

			continue
		}

		// Or the argument only asks for a limited number
		// of words and we have too many of them.
		if arg.Maximum != -1 && arg.Value.Len() > arg.Maximum {
			names = append(names, argHasTooMany(*arg))
		}
	}

	return names
}

// makes a correct sentence when we don't have enough args.
func argHasNotEnough(arg positional.Arg) string {
	var arguments string

	if arg.Minimum > 1 {
		arguments = "arguments, but got only " + fmt.Sprintf("%d", arg.Value.Len())
	} else {
		arguments = argumentWordReq
	}

	argRequired := "`" + arg.Name + " (at least " + fmt.Sprintf("%d",
		arg.Minimum) + " " + arguments + ")`"

	return argRequired
}

// makes a correct sentence when we have too much args.
func argHasTooMany(arg positional.Arg) string {
	// The argument might be explicitly disabled...
	if arg.Maximum == 0 {
		return "`" + arg.Name + " (zero arguments)`"
	}

	// Or just build the list accordingly.
	var parsed string

	if arg.Maximum > 1 {
		parsed = "arguments, but got " + fmt.Sprintf("%d", arg.Value.Len())
	} else {
		parsed = argumentWordReq
	}

	hasTooMany := "`" + arg.Name + " (at most " + fmt.Sprintf("%d", arg.Maximum) + " " + parsed + ")`"

	return hasTooMany
}
