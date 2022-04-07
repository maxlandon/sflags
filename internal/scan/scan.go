package scan

import (
	"errors"
	"reflect"

	"github.com/octago/sflags/internal/tag"
)

// ErrNotPointerToStruct indicates that a provided data container is not
// a pointer to a struct. Only pointers to structs are valid data containers
// for options.
var ErrNotPointerToStruct = errors.New("object must be a pointer to struct or interface")

// Handler is a generic handler used for scanning both commands and group structs alike.
type Handler func(reflect.Value, *reflect.StructField) (bool, error)

// Type actually scans the type, recursively if needed.
func Type(data interface{}, handler Handler) error {
	// Get all the public fields in the data struct
	ptrval := reflect.ValueOf(data)

	if ptrval.Type().Kind() != reflect.Ptr {
		return ErrNotPointerToStruct
	}

	stype := ptrval.Type().Elem()

	if stype.Kind() != reflect.Struct {
		return ErrNotPointerToStruct
	}

	realval := reflect.Indirect(ptrval)

	if err := scanStruct(realval, nil, handler); err != nil {
		return err
	}

	return nil
}

// scanStruct performs an exhaustive scan of a struct that we found as field (embedded),
// either with the specified scanner, or manually -in which case we will recursively scan
// embedded structs themselves.
func scanStruct(val reflect.Value, sfield *reflect.StructField, scan Handler) error {
	stype := val.Type()

	// We are being passed a field only when a have a "root struct"
	// already being parsed, a kind of reference point. It can be
	// either for scanning for a subcommand, a group of options,
	// or even a group of subcommands.
	if sfield != nil {
		if ok, err := scan(val, sfield); err != nil {
			return err
		} else if ok {
			return nil
		}
	}

	// But most of the time we end up here, and look each field again.
	for fieldCount := 0; fieldCount < stype.NumField(); fieldCount++ {
		field := stype.Field(fieldCount)

		// Scan the field for either a subgroup (if the field is a struct)
		// or for an option. Any error cancels the scan and is immediately returned.
		if err := scanField(val, fieldCount, field, scan); err != nil {
			return err
		}
	}

	return nil
}

// scanField attempts to grab a tag on a struct field, and depending on the field's type,
// either scans recursively if the field is an embedded struct/pointer, or attempts to scan
// the field as an option of the group.
func scanField(val reflect.Value, fCount int, field reflect.StructField, scan Handler) error {
	// Get the field tag and return/continue if failed/needed
	_, skip, err := tag.GetFieldTag(field)
	if err != nil {
		return err
	} else if skip {
		return nil
	}

	fld := val.Field(fCount)
	kind := field.Type.Kind()

	// Either the embedded fied is a struct value
	if field.Type.Kind() == reflect.Struct {
		if err := scanStruct(fld, &field, scan); err != nil {
			return err
		}
	}

	// Or the embedded field is a pointer to a struct, ensure there's no nil
	if kind == reflect.Ptr && field.Type.Elem().Kind() == reflect.Struct {
		if fld.IsNil() {
			fld = reflect.New(fld.Type().Elem())
		}

		if err := scanStruct(reflect.Indirect(fld), &field, scan); err != nil {
			return err
		}
	}

	// We're done with this field regardless of what we actually did with it.
	return nil
}