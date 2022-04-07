package positional

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/octago/sflags/internal/tag"
)

// Arg is a type used to store information and value references to
// a struct field we use as positional arg. This type is passed in
// many places, so that we can parse/convert and make informed
// decisions on how to handle those tasks.
type Arg struct {
	Index    int           // The position in the struct (n'th struct field used as a slot)
	Name     string        // name of the argument, either tag name or struct field
	Minimum  int           // minimum number of arguments we want.
	Maximum  int           // Maximum number of args we want (-1: infinite)
	StartMin int           // Index of first positional word for which we are used
	StartMax int           // if previous positional slots are full, this replaces startAt
	Tag      tag.MultiTag  // struct tag
	Value    reflect.Value // A reference to the field value itself
}

// makePositionals scans an entire value (must be ensured to be a struct) and
// creates a list of positional arguments, along with many required minimum total
// number of arguments we need. Any non-nil error ends the scan, no matter where.
func Scan(val reflect.Value, reqAll bool) ([]*Arg, int, error) {
	// The real type of the struct we're scanning
	stype := val.Type()

	var positionals []*Arg

	reqTotal := 0 // Total count of required arguments
	reqMax := 0   // the maximum number of required arguments

	// Each positional field is scanned for its number requirements,
	// and underlying value to be used by the command's arg handlers/converters.
	for fieldCount := 0; fieldCount < stype.NumField(); fieldCount++ {
		field := stype.Field(fieldCount)
		fieldValue := val.Field(fieldCount)

		ptag, name, err := parsePositionalTag(field)
		if err != nil {
			return positionals, reqTotal, err
		}

		// Set min/max requirements depending on the tag, the overall
		// requirement settings (at struct level), also taking into
		// account the kind of field we are considering (slice or not)
		min, max := positionalReqs(fieldValue, ptag, reqAll)

		arg := &Arg{
			Index:    len(positionals),
			Name:     name,
			Minimum:  min,
			Maximum:  max,
			Tag:      ptag,
			StartMin: reqTotal,
			StartMax: reqMax,
			Value:    fieldValue,
		}

		positionals = append(positionals, arg)
		reqTotal += min // min is never < 0

		// The total maximum number of arguments is used
		// by completers to know precisely when they should
		// start completing for a given positional field slot.
		if arg.Maximum != -1 {
			reqMax += arg.Maximum
		}
	}

	// Depending on our position and type, we reset the maximum
	// number of words allowed for this argument, and update the
	// counter that will be used by handlers to sync their use
	// of words
	for _, arg := range positionals {
		adjustMaximums(positionals, arg)
	}

	return positionals, reqTotal, nil
}

// positionalReqs determines the correct quantity requirements for a positional field,
// depending on its parsed struct tag values, and the underlying type of the field.
func positionalReqs(val reflect.Value, mtag tag.MultiTag, all bool) (min, max int) {
	required, max := parseArgsNumRequired(mtag)

	// When the argument field is not a slice, we have to adjust for some defaults
	isSlice := val.Type().Kind() == reflect.Slice || val.Type().Kind() == reflect.Map

	if required > 0 && isSlice {
		// If a slice has at least one required, add this minimum
		// Increase the total number of positional args wanted.
		min += required
	} else if required > 0 || (all && required == -1) {
		// When the argument is marked not required, just
		// require one, regardless if it can accept more.
		min++
	}

	return min, max
}

// adjustMaximums analyzes the position of a positional argument field,
// and adjusts its maximum so that handlers can work on them correctly.
func adjustMaximums(positionals []*Arg, arg *Arg) {
	val := arg.Value
	isSlice := val.Type().Kind() == reflect.Slice ||
		val.Type().Kind() == reflect.Map

	// First, the maximum index at which we should start
	// parsing words can never be smaller than the minimum one
	if arg.StartMax < arg.StartMin {
		arg.StartMax = arg.StartMin
	}

	// The maximum is not left to -1 under some conditions:
	// The field is unique, but required, so we want only one.
	if arg.Maximum == -1 && !isSlice {
		arg.Maximum = 1

		return
	}

	// If we are the last, normally there is nothing to adjust for,
	// especially the maximum -1 that is important if it was set.
	if arg.Index == len(positionals)-1 {
		return
	}

	// Else we're not the last argument, so we set the maximum
	// to our minimum only if both are different
	// if isSlice && arg.max == -1 {
	//         arg.max = arg.min
	// }
}

// parsePositionalTag extracts and fully parses a struct (positional) field tag.
func parsePositionalTag(field reflect.StructField) (tag.MultiTag, string, error) {
	tag, none, err := tag.GetFieldTag(field)
	if none || err != nil {
		return tag, "", err
	}

	name, _ := tag.Get("positional-arg-name")

	if len(name) == 0 {
		name = field.Name
	}

	return tag, name, nil
}

// parseArgsNumRequired sets the minimum/maximum requirements for an argument field.
func parseArgsNumRequired(fieldTag tag.MultiTag) (required, maximum int) {
	required = -1
	maximum = -1

	sreq, _ := fieldTag.Get("required")

	// If no requirements, -1 means unlimited
	if sreq == "" {
		return
	}

	required = 1

	rng := strings.SplitN(sreq, "-", requiredNumParsedValues)

	if len(rng) > 1 {
		if preq, err := strconv.ParseInt(rng[0], baseParseInt, bitsizeParseInt); err == nil {
			required = int(preq)
		}

		if preq, err := strconv.ParseInt(rng[1], baseParseInt, bitsizeParseInt); err == nil {
			maximum = int(preq)
		}
	} else {
		if preq, err := strconv.ParseInt(sreq, baseParseInt, bitsizeParseInt); err == nil {
			required = int(preq)
		}
	}

	return required, maximum
}
