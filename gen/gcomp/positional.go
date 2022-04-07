package gcomp

import (
	"reflect"
	"sync"

	comp "github.com/rsteube/carapace"

	"github.com/octago/sflags/internal/positional"
	"github.com/octago/sflags/internal/tag"
)

// positionals finds a struct tagged as containing positional arguments and scans them.
func positionals(comps *comp.Carapace, tag tag.MultiTag, val reflect.Value) (bool, error) {
	// We need the struct to be marked as such
	if pargs, _ := tag.Get("positional-args"); len(pargs) == 0 {
		return false, nil
	}

	req, _ := tag.Get("required") // this is written on the struct, applies to all
	reqAll := len(req) != 0       // Each field will count as one required minimum

	// Scan all the fields on the struct and build the list of arguments
	// with their own requirements, and references to their values.
	positionals, reqTotal, err := positional.Scan(val, reqAll)
	if err != nil {
		return true, err
	}

	// Find all completer implementations, or
	// build ones based on struct tag specs.
	// Put them in a cache of completion callbacks that is accessed
	// by all positional arguments in order to use their completions.
	completionCache := getCompleters(positionals, comps)

	// Once we a have a list of positionals, completers for each,
	// and the number of arguments required, we can build a single
	// completion handler, similar to our ValidArgs function handler
	handler := positionalCompleter(positionals, completionCache, reqTotal)

	// And bind this positional completer to our command
	comps.PositionalAnyCompletion(comp.ActionCallback(handler))

	return true, nil
}

// getCompleters populates the completers for each positional argument in
// a list of them, through either implemented methods or struct tag specs.
func getCompleters(args []*positional.Arg, comps *comp.Carapace) *compCache {
	// The cache stores all completer functions, to be used later.
	cache := newCompletionCache()

	for _, arg := range args {
		// Make parser function, get completer implementations, how many arguments, etc.
		if completer := typeCompleter(arg.Value); completer != nil {
			cache.add(arg.Index, completer)

			// Always overwrite the after-dash completion if this argument field is
			// being indicated as such through its struct tag.
			if isDashPositionalAny(arg.Tag) {
				comps.DashAnyCompletion(comp.ActionCallback(completer))
			}
		}

		// But struct tags have precedence, so here should take place
		// most of the work, since it's quite easy to specify powerful completions.
		if completer, found := taggedCompletions(arg.Tag); found {
			cache.add(arg.Index, completer)
		}
	}

	return cache
}

// positionalCompleter builds a handler that is used to concurrently analyze
// the positional words of the command line, and to propose completions with the
// most educated guess possible: arguments know a lot, and they can even try to
// actually convert their values to verify if their type/value is good.
func positionalCompleter(args []*positional.Arg, cache *compCache, needed int) comp.CompletionCallback {
	handler := func(ctx comp.Context) comp.Action {
		// All positionals work concurrently
		compWorkers := &sync.WaitGroup{}

		for _, arg := range args {
			compWorkers.Add(1)

			// Each argument processes a copy of the words concurently.
			go func(arg *positional.Arg, wg *sync.WaitGroup) {
				defer wg.Done()

				// If we don't have enough words for even
				// considering this positional to be completed.
				if len(ctx.Args) < arg.StartMin {
					return
				}

				// Each positional makes a stack with a copy
				// of the arguments It knows where to start.
				words := positional.GetWords(*arg, ctx.Args, needed)

				// The argument will loop over all the argument words
				if err := consumeWords(arg, words, cache); err != nil {
					// An error is often unrecoverable, so we should
					// probably break and populate the completions with
					// the appropriate error message.
					// TODO: error message to comps
					return
				}
			}(arg, compWorkers)
		}

		// Wait until all of our positional arguments have
		// processed all of the(ir) words, regardless of if
		// they produced some completion.
		compWorkers.Wait()

		// We are done processing some/all of the positional words.
		// The cache contains all the completions it needs, so we
		// just unload them into one action to be returned
		return cache.flush(ctx)
	}

	return handler
}

// consumeWords is called on each positional argument, so that it can consume
// one/more of the positional words and add completions to the cache if needed.
func consumeWords(arg *positional.Arg, stack *positional.Words, comps *compCache) error {
	// Always complete if we have no maximum
	if arg.Maximum == -1 {
		return completeOrIgnore(arg, comps, 0)
	}

	// If there is a drift between the accumulated words and
	// the maximum requirements of the PREVIOUS positionals,
	// we use this drift in order not to pop the words as soon
	// as we would otherwise do. Useful when more than one positional
	// arguments have a minimum-maximum range of allowed arguments.
	drift := arg.StartMax - arg.StartMin
	actuallyParsed := 0

	// As long as we've got a word, and nothing told us to quit.
	for !stack.Empty() {
		if drift == 0 {
			// That we either consider to be parsed by
			// our current positional slot, we pop an
			// argument that should be parsed by us.
			actuallyParsed++
		} else if drift > 0 {
			// Or to be left to one of the preceding
			// positionals, which have still some slots
			// available for arguments.
			drift--
		}

		// Pop the next positional word, as if we would
		// parse/convert it into our slot at exec time.
		stack.Pop()

		// If we have reached the maximum number
		// of args we accept, don't complete
		if arg.Maximum == actuallyParsed {
			break
		}
	}

	// This function makes the final call on whether to
	// complete for this positional or not.
	return completeOrIgnore(arg, comps, actuallyParsed)
}

// completeOrIgnore finally takes the decision of completing this positional or not.
func completeOrIgnore(arg *positional.Arg, comps *compCache, actuallyParsed int) error {
	mustComplete := false

	switch {
	case arg.Maximum == -1:
		// Always complete if we have no maximum
		mustComplete = true
	case actuallyParsed < arg.Minimum:
		// If we are still lacking some required words,
		// but we have exhausted the available ones.
		mustComplete = true
	case actuallyParsed < arg.Maximum:
		// Or we have the minimum required, but we could
		// take more.
		mustComplete = true
	}

	// If something has said we must, cache the comps.
	if mustComplete {
		comps.useCompleter(arg.Index)
	}

	return nil
}

func isDashPositionalAny(tag tag.MultiTag) bool {
	isDashAny, _ := tag.Get("complete")

	// TODO: here extract all complete directives

	return isDashAny != ""
}

// a list used to store completion callbacks produced by our
// positional arguments' slots at some point in the process.
type compCache struct {
	// All positionals have given their completers
	// before running, so we can access them
	completers *map[int]comp.CompletionCallback
	// And the cache is the list of completion callbacks
	// we will actually use when exiting the full process.
	cache []comp.CompletionCallback
}

func newCompletionCache() *compCache {
	return &compCache{
		completers: &map[int]comp.CompletionCallback{},
	}
}

func (c *compCache) add(index int, cb comp.CompletionCallback) {
	(*c.completers)[index] = cb
}

func (c *compCache) useCompleter(index int) {
	completer, found := (*c.completers)[index]
	if found {
		c.cache = append(c.cache, completer)
	}
}

// flush returns all the completions cached by our positional arguments,
// so we invoke each of them with the context so that they can perform
// so filtering tasks if they need to.
func (c *compCache) flush(ctx comp.Context) (action comp.Action) {
	actions := make([]comp.Action, 0)

	// fixed-max positional completers
	for _, cb := range c.cache {
		actions = append(actions, comp.ActionCallback(cb))
	}

	// Each of the completers should invoke with
	// the context so that they can filter out
	// the candidates that are already present.
	processed := make([]comp.Action, 0)

	for _, completion := range actions {
		completion = completion.Invoke(ctx).Filter(ctx.Args).ToA()
		processed = append(processed, completion)
	}

	// Let carapace merge all of our callbacks.
	return comp.Batch(processed...).ToA()
}
