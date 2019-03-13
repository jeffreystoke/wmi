// +build windows

package wmi

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// Unmarshaler is the interface implemented by types that can unmarshal COM
// object of themselves.
type Unmarshaler interface {
	UnmarshalOLE(src *ole.IDispatch) error
}

// Decoder handles "decoding" of `ole.IDispatch` objects into the given
// structure. See `Decoder.Unmarshal` for more info.
type Decoder struct {
	// NonePtrZero specifies if nil values for fields which aren't pointers
	// should be returned as the field types zero value.
	//
	// Setting this to true allows stucts without pointer fields to be used
	// without the risk failure should a nil value returned from WMI.
	NonePtrZero bool

	// PtrNil specifies if nil values for pointer fields should be returned
	// as nil.
	//
	// Setting this to true will set pointer fields to nil where WMI
	// returned nil, otherwise the types zero value will be returned.
	PtrNil bool

	// AllowMissingFields specifies that struct fields not present in the
	// query result should not result in an error.
	//
	// Setting this to true allows custom queries to be used with full
	// struct definitions instead of having to define multiple structs.
	AllowMissingFields bool
}

// ErrFieldMismatch is returned when a field is to be loaded into a different
// type than the one it was stored from, or when a field is missing or
// unexported in the destination struct.
// FieldType is the type of the struct pointed to by the destination argument.
type ErrFieldMismatch struct {
	FieldType reflect.Type
	FieldName string
	Reason    string
}

func (e ErrFieldMismatch) Error() string {
	return fmt.Sprintf("wmi: cannot load field %q into a %q: %s",
		e.FieldName, e.FieldType, e.Reason)
}

var timeType = reflect.TypeOf(time.Time{})

// Unmarshal loads `ole.IDispatch` into a struct pointer.
// N.B. Unmarshal supports only limited subset of structure field
// types:
// - all signed and unsigned integers
// - time.Time
// - string
// - bool
// - float32
// - a pointer to one of types above
// - a slice of one of thus types
// - structure types.
//
// To unmarshal more complex struct consider implementing `wmi.Unmarshaler`.
// For such types Unmarshal just calls `.UnmarshalOLE` on the @src object .
//
// To unmarshal COM-object into a struct, Unmarshal tries to fetch COM-object
// properties for each public struct field using as a property name either
// field name itself or the name specified in "wmi" field tag.
//
// By default any field missed in the COM-object leads to the error. To allow
// skipping such fields set `.AllowMissingFields` to `true`.
//
// Unmarshal does some "smart" type conversions between integer types (including
// unsigned ones), so you could receive e.g. `uint32` into `uint` if you don't
// care about the size.
//
// Unmarshal allows to specify special COM-object property name or skip a field
// using structure field tags, e.g.
//    // Will be filled from property `Frequency_Object`
//    FrequencyObject int wmi:"Frequency_Object"`
//
//    // Will be skipped during unmarshalling.
//    MyHelperField   int wmi:"-"`
//
// Unmarshal prefers tag value over the field name, but ignores any name collisions.
// So for example all the following fields will be resolved to the same value.
//    Field  int
//    Field1 int `wmi:"Field"`
//    Field2 int `wmi:"Field"`
func (d *Decoder) Unmarshal(src *ole.IDispatch, dst interface{}) (err error) {
	defer func() {
		// We use lots of reflection, so always be alert!
		if r := recover(); r != nil {
			err = fmt.Errorf("runtime panic: %v", err)
		}
	}()

	// Checks whether the type can handle unmarshalling of himself.
	if u, ok := dst.(Unmarshaler); ok {
		return u.UnmarshalOLE(src)
	}

	v := reflect.ValueOf(dst).Elem()
	vType := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		fType := vType.Field(i)
		fieldName := getFieldName(fType)

		if !f.CanSet() || fieldName == "-" {
			continue
		}

		// Closure for defer in the loop.
		err = func() error {
			// Fetch property from the COM object.
			prop, err := oleutil.GetProperty(src, fieldName)
			if err != nil {
				if !d.AllowMissingFields {
					return errors.New("no such result field")
				}
				return nil // TODO: Is it really ok?
			}
			defer prop.Clear()
			if prop.VT == ole.VT_NULL {
				return nil
			}
			return d.unmarshalField(f, prop)
		}()
		if err != nil {
			return ErrFieldMismatch{
				FieldType: fType.Type,
				FieldName: fieldName,
				Reason:    err.Error(),
			}
		}
	}

	return nil
}

func (d *Decoder) unmarshalField(fieldDst reflect.Value, prop *ole.VARIANT) error {
	isPtr := fieldDst.Kind() == reflect.Ptr
	fieldDstOrig := fieldDst
	if isPtr { // Create empty object for pointer receiver.
		ptr := reflect.New(fieldDst.Type().Elem())
		fieldDst.Set(ptr)
		fieldDst = fieldDst.Elem()
	}

	// First of all try to unmarshal it as a simple type.
	err := unmarshalSimpleValue(fieldDst, prop.Value())
	if err != errSimpleVariantsExceeded {
		return err // Either nil and value set or unexpected error.
	}

	// Or we faced not so simple type. Do our best.
	switch fieldDst.Kind() {
	case reflect.Slice:
		safeArray := prop.ToArray()
		if safeArray == nil {
			return fmt.Errorf("can't unmarshal %s into slice", prop.VT)
		}
		return unmarshalSlice(fieldDst, safeArray)
	case reflect.Struct:
		dispatch := prop.ToIDispatch()
		if dispatch == nil {
			return fmt.Errorf("can't unmarshal %s into struct", prop.VT)
		}
		fieldPointer := fieldDst.Addr().Interface()
		return d.Unmarshal(dispatch, fieldPointer)
	default:
		// If we got nil value - handle it with magic config fields.
		gotNilProp := reflect.TypeOf(prop.Value()) == nil
		if gotNilProp && (isPtr || d.NonePtrZero) {
			ptrNeedZero := isPtr && d.PtrNil
			nonPtrAllowNil := !isPtr && d.NonePtrZero
			if ptrNeedZero || nonPtrAllowNil {
				fieldDstOrig.Set(reflect.Zero(fieldDstOrig.Type()))
			}
			return nil
		}
		return fmt.Errorf("unsupported type (%T)", prop.Value())
	}
}

var (
	errSimpleVariantsExceeded = errors.New("unknown simple type")
)

// Here goes some kind of "smart" (too smart) field unmarshalling.
// It checks a type of a property value returned from COM object and then
// tries to fit it inside a given structure field with some possible
// conversions (e.g. possible integer conversions, string to int parsing
// and others).
func unmarshalSimpleValue(dst reflect.Value, value interface{}) error {
	switch val := value.(type) {
	case int8, int16, int32, int64, int:
		v := reflect.ValueOf(val).Int()
		switch dst.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			dst.SetInt(v)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			dst.SetUint(uint64(v))
		default:
			return errors.New("not an integer class")
		}
	case uint8, uint16, uint32, uint64:
		v := reflect.ValueOf(val).Uint()
		switch dst.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			dst.SetInt(int64(v))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			dst.SetUint(v)
		default:
			return errors.New("not an integer class")
		}
	case bool:
		switch dst.Kind() {
		case reflect.Bool:
			dst.SetBool(val)
		default:
			return errors.New("not a bool")
		}
	case float32:
		switch dst.Kind() {
		case reflect.Float32:
			dst.SetFloat(float64(val))
		default:
			return errors.New("not a float32")
		}
	case string:
		return smartUnmarshalString(dst, val)
	default:
		return errSimpleVariantsExceeded
	}
	return nil
}

func unmarshalSlice(fieldDst reflect.Value, safeArray *ole.SafeArrayConversion) error {
	arr := safeArray.ToValueArray()
	resultArr := reflect.MakeSlice(fieldDst.Type(), len(arr), len(arr))
	for i, v := range arr {
		s := resultArr.Index(i)
		err := unmarshalSimpleValue(s, v)
		if err != nil {
			return fmt.Errorf("can't put %T into []%s", v, fieldDst.Type().Elem().Kind())
		}
	}
	fieldDst.Set(resultArr)
	return nil
}

func smartUnmarshalString(fieldDst reflect.Value, val string) error {
	switch fieldDst.Kind() {
	case reflect.String:
		fieldDst.SetString(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		iv, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return err
		}
		fieldDst.SetInt(iv)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		uv, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return err
		}
		fieldDst.SetUint(uv)
	case reflect.Struct:
		switch t := fieldDst.Type(); t {
		case timeType:
			return unmarshalTime(fieldDst, val)
		default:
			return fmt.Errorf("can't deserialize string into struct %T", fieldDst.Interface())
		}
	default:
		return fmt.Errorf("can't deserealize string into %s", fieldDst.Kind())
	}
	return nil
}

func unmarshalTime(fieldDst reflect.Value, val string) error {
	if len(val) == 25 {
		mins, err := strconv.Atoi(val[22:])
		if err != nil {
			return err
		}
		val = val[:22] + fmt.Sprintf("%02d%02d", mins/60, mins%60)
	}
	t, err := time.Parse("20060102150405.000000-0700", val)
	if err != nil {
		return err
	}
	fieldDst.Set(reflect.ValueOf(t))
	return nil
}

func getFieldName(fType reflect.StructField) string {
	tag := fType.Tag.Get("wmi")
	if tag != "" {
		return tag
	}
	return fType.Name
}
