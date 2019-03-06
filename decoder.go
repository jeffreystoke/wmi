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

// Decoder handles "decoding" of `ole.IDispatch`objects into the given
// structure. N.B. Decoder supports only limited subset of structure field
// types:
// - all signed and unsigned integers
// - time.Time
// - string
// - bool
// - a pointer to one of types above
// - []string and []byte.
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

// Unmarshal loads `ole.IDispatch` into a struct pointer. See `Decoder` for
// more info.
func (d *Decoder) Unmarshal(src *ole.IDispatch, dst interface{}) (err error) {
	defer func() {
		// We use lots of reflection, so always be alert!
		if r := recover(); r != nil {
			err = fmt.Errorf("runtime panic: %v", err)
		}
	}()

	v := reflect.ValueOf(dst).Elem()
	vType := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !f.CanSet() {
			continue
		}
		fType := vType.Field(i)
		fieldName := fType.Name

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

	// Then goes some kind of "smart" (too smart) field unmarshaling.
	// It checks a type of a property value returned from COM object and then
	// tries to fit it inside a given structure field with some possible
	// conversions (e.g. possible integer conversions, string to int parsing
	// and others).

	switch val := prop.Value().(type) {
	case int8, int16, int32, int64, int:
		v := reflect.ValueOf(val).Int() // TODO: is it really necessary?
		switch fieldDst.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fieldDst.SetInt(v)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			fieldDst.SetUint(uint64(v))
		default:
			return errors.New("not an integer class")
		}
	case uint8, uint16, uint32, uint64:
		v := reflect.ValueOf(val).Uint()
		switch fieldDst.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			fieldDst.SetInt(int64(v))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			fieldDst.SetUint(v)
		default:
			return errors.New("not an integer class")
		}
	case bool:
		switch fieldDst.Kind() {
		case reflect.Bool:
			fieldDst.SetBool(val)
		default:
			return errors.New("not a bool")
		}
	case float32:
		switch fieldDst.Kind() {
		case reflect.Float32:
			fieldDst.SetFloat(float64(val))
		default:
			return errors.New("not a float32")
		}
	case string:
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
			default:
				return fmt.Errorf("can't deserealize string into %s", t)
			}
		}
	default:
		if fieldDst.Kind() == reflect.Slice {
			switch fieldDst.Type().Elem().Kind() {
			case reflect.String:
				safeArray := prop.ToArray()
				if safeArray != nil {
					arr := safeArray.ToValueArray()
					fArr := reflect.MakeSlice(fieldDst.Type(), len(arr), len(arr))
					for i, v := range arr {
						s := fArr.Index(i)
						s.SetString(v.(string))
					}
					fieldDst.Set(fArr)
				}
			case reflect.Uint8:
				safeArray := prop.ToArray()
				if safeArray != nil {
					arr := safeArray.ToValueArray()
					fArr := reflect.MakeSlice(fieldDst.Type(), len(arr), len(arr))
					for i, v := range arr {
						s := fArr.Index(i)
						s.SetUint(reflect.ValueOf(v).Uint())
					}
					fieldDst.Set(fArr)
				}
			default:
				return fmt.Errorf("unsupported slice type (%T)", val)
			}
		} else {
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
			return fmt.Errorf("unsupported type (%T)", val)
		}
	}
	return nil
}
