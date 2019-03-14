// +build windows

package wmi

import (
	"errors"
	"fmt"
	"github.com/hashicorp/go-multierror"
	"github.com/scjalliance/comshim"
	"reflect"
	"sync"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// SWbemServicesConnection is used to access SWbemServices methods of the
// single server.
//
// Ref: https://docs.microsoft.com/en-us/windows/desktop/wmisdk/swbemservices
type SWbemServicesConnection struct {
	sync.Mutex
	Decoder

	sWbemServices    *ole.IDispatch // This is for calls.
	SWbemServicesRaw *ole.VARIANT   // This is for `.Clear()`.
}

func ConnectSWbemServices(connectServerArgs ...interface{}) (conn *SWbemServicesConnection, err error) {
	services, err := NewSWbemServices()
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := services.Close(); closeErr != nil {
			err = multierror.Append(err, closeErr)
		}
	}()
	return services.ConnectServer(connectServerArgs...)
}

func (s *SWbemServices) ConnectServer(args ...interface{}) (c *SWbemServicesConnection, err error) {
	// Notify that we are going to use COM. We will care about at least one
	// reference for connection.
	comshim.Add(1)
	defer func() {
		if err != nil {
			comshim.Done()
		}
	}()

	serviceRaw, err := oleutil.CallMethod(s.sWbemLocator, "ConnectServer", args...)
	if err != nil {
		return nil, fmt.Errorf("SWbemServices ConnectServer error; %v", err)
	}
	service := serviceRaw.ToIDispatch()
	if service == nil {
		return nil, errors.New("SWbemServices IDispatch returned nil")
	}

	conn := SWbemServicesConnection{
		Decoder:          s.Decoder,
		sWbemServices:    service,
		SWbemServicesRaw: serviceRaw,
	}
	return &conn, nil
}

// Close will clear and release all of the SWbemServicesConnection resources.
func (s *SWbemServicesConnection) Close() error {
	s.Lock()
	defer s.Unlock()
	if s.sWbemServices == nil {
		return nil
	}
	err := s.SWbemServicesRaw.Clear()
	if err != nil {
		return err
	}
	s.SWbemServicesRaw = nil
	s.sWbemServices.Release()
	s.sWbemServices = nil
	comshim.Done()
	return nil
}

// Query runs the WQL query using a SWbemServicesConnection instance and appends
// the values to dst.
func (s *SWbemServicesConnection) Query(query string, dst interface{}) error {
	s.Lock()
	if s.sWbemServices == nil {
		s.Unlock()
		return fmt.Errorf("SWbemServicesConnection has been closed")
	}
	s.Unlock()

	sliceRefl := reflect.ValueOf(dst)
	if sliceRefl.Kind() != reflect.Ptr || sliceRefl.IsNil() {
		return ErrInvalidEntityType
	}
	sliceRefl = sliceRefl.Elem() // "Dereference" pointer.

	argType, elemType := checkMultiArg(sliceRefl)
	if argType == multiArgTypeInvalid {
		return ErrInvalidEntityType
	}

	return s.query(query, &queryDst{
		dst:         sliceRefl,
		dsArgType:   argType,
		dstElemType: elemType,
	})
}

type queryDst struct {
	dst         reflect.Value
	dsArgType   multiArgType
	dstElemType reflect.Type
}

func (s *SWbemServicesConnection) query(query string, dst *queryDst) (err error) {
	// result is a SWBemObjectSet
	resultRaw, err := oleutil.CallMethod(s.sWbemServices, "ExecQuery", query)
	if err != nil {
		return err
	}
	result := resultRaw.ToIDispatch()
	defer func() {
		if clErr := resultRaw.Clear(); clErr != nil {
			err = multierror.Append(err, clErr)
		}
	}()

	count, err := oleInt64(result, "Count")
	if err != nil {
		return err
	}

	enumProperty, err := result.GetProperty("_NewEnum")
	if err != nil {
		return err
	}
	defer func() {
		if clErr := enumProperty.Clear(); clErr != nil {
			err = multierror.Append(err, clErr)
		}
	}()

	enum, err := enumProperty.ToIUnknown().IEnumVARIANT(ole.IID_IEnumVariant)
	if err != nil {
		return err
	}
	if enum == nil {
		return fmt.Errorf("can't get IEnumVARIANT, enum is nil")
	}
	defer enum.Release()

	// Initialize a slice with Count capacity
	dst.dst.Set(reflect.MakeSlice(dst.dst.Type(), 0, int(count)))

	var errFieldMismatch error
	for itemRaw, length, err := enum.Next(1); length > 0; itemRaw, length, err = enum.Next(1) {
		if err != nil {
			return err
		}

		// Closure for defer in the loop.
		err := func() error {
			// item is a SWbemObject, but really a Win32_Process
			item := itemRaw.ToIDispatch()
			defer item.Release()

			ev := reflect.New(dst.dstElemType)
			if err = s.Unmarshal(item, ev.Interface()); err != nil {
				if _, ok := err.(*ErrFieldMismatch); ok {
					// We continue loading entities even in the face of field mismatch errors.
					// If we encounter any other error, that other error is returned. Otherwise,
					// an ErrFieldMismatch is returned.
					errFieldMismatch = multierror.Append(errFieldMismatch, err)
				} else {
					return err
				}
			}

			if dst.dsArgType != multiArgTypeStructPtr {
				ev = ev.Elem()
			}
			dst.dst.Set(reflect.Append(dst.dst, ev))

			return nil
		}()
		if err != nil {
			return err
		}
	}
	return errFieldMismatch
}

type multiArgType int

const (
	multiArgTypeInvalid multiArgType = iota
	multiArgTypeStruct
	multiArgTypeStructPtr
)

// checkMultiArg checks that v has type []S, []*S for some struct type S.
//
// It returns what category the slice's elements are, and the reflect.Type
// that represents S.
func checkMultiArg(v reflect.Value) (m multiArgType, elemType reflect.Type) {
	if v.Kind() != reflect.Slice {
		return multiArgTypeInvalid, nil
	}
	elemType = v.Type().Elem()
	switch elemType.Kind() {
	case reflect.Struct:
		return multiArgTypeStruct, elemType
	case reflect.Ptr:
		elemType = elemType.Elem()
		if elemType.Kind() == reflect.Struct {
			return multiArgTypeStructPtr, elemType
		}
	}
	return multiArgTypeInvalid, nil
}

func oleInt64(item *ole.IDispatch, prop string) (val int64, err error) {
	v, err := oleutil.GetProperty(item, prop)
	if err != nil {
		return 0, err
	}
	defer func() {
		if clErr := v.Clear(); clErr != nil {
			err = multierror.Append(err, clErr)
		}
	}()

	i := int64(v.Val)
	return i, nil
}
