package positional

// Words is used to easily manage our positional arguments
// passed by a cobra.Command to its Args handlers/completers.
type Words struct {
	list   []string // The list of arguments remaining to be parsed into their fields
	done   *int     // A pointer that is being shared by all positional argument handlers
	parsed int      // A counter used only by a single positional field
	needed int      // A global value set when we know the total number of arguments
}

// MakeWords makes a new stack of arguments to be used by positional
// handlers. The startAt index is an indication of where we should start
// in the arguments, as cobra commands always pass the entire array of
// positionals, and we have to sort out which have been parsed already.
func MakeWords(args []string, startAt *int, needed int) *Words {
	return &Words{
		list:   args[*startAt:],
		done:   startAt,
		parsed: 0,
		needed: needed,
	}
}

// getWords forms a stack that starts at the index this positional
// is supposed to start accepting words, not before. Used for comps.
func GetWords(arg Arg, args []string, needed int) *Words {
	words := &Words{
		list:   make([]string, 0),
		done:   &arg.StartMin,
		parsed: 0,
		needed: needed,
	}

	if len(args) > arg.StartMin {
		words.list = args[arg.StartMin:]
	}

	return words
}

// List returns a copy of the words.
func (words *Words) List() []string {
	return words.list
}

// Len returns the number of positional
// words we still haven't parsed/used.
func (words *Words) Len() int {
	return len(words.list)
}

// Empty returns true if there are no more words.
func (words *Words) Empty() bool {
	return len(words.list) == 0
}

// Required returns the total minimum number of words
// we want in order to satisfy ALL of the positional
// argument slots. Set at list init time.
func (words *Words) Required() int {
	return words.needed
}

// Done returns the number of arguments we have
// popped and/or parsed. Set at list init time.
func (words *Words) DoneAll() int {
	return *words.done
}

// Parsed returns the number of words this stack only
// have parsed, which is generally smaller than words.Done()
// (the latter is set with a pointer passed by the previous
// positional, at setup time).
func (words *Words) Parsed() int {
	return words.parsed
}

// Pop returns the first word in the words
// list, and remove this word from the list.
// Also updates the various counters in list.
func (words *Words) Pop() string {
	if words.Empty() {
		return ""
	}

	// Update the list of arguments
	arg := words.list[0]
	words.list = words.list[1:]

	// Update the individual counter
	// used by our argument field handler
	words.parsed++

	// And set our counter, so that
	// all other handlers can see it
	*words.done++

	return arg
}
