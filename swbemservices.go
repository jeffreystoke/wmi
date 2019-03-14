// +build windows

package wmi

import (
	"fmt"
	"github.com/hashicorp/go-multierror"
	"github.com/scjalliance/comshim"
	"sync"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// SWbemServices is used to access wmi.
// Ref: https://docs.microsoft.com/en-us/windows/desktop/wmisdk/swbemservices
type SWbemServices struct {
	sync.Mutex
	Decoder

	sWbemLocator    *ole.IDispatch // This is for calls.
	sWbemLocatorRaw *ole.IUnknown  // This is for `.Clear()`.
}

func NewSWbemServices() (s *SWbemServices, err error) {
	comshim.Add(1)
	defer func() {
		if err != nil {
			comshim.Done()
		}
	}()

	locatorIUnknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, fmt.Errorf("CreateObject SWbemLocator erro; %v", err)
	} else if locatorIUnknown == nil {
		return nil, ErrNilCreateObject
	}

	sWbemLocator, err := locatorIUnknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, fmt.Errorf("SWbemLocator QueryInterface error; %v", err)
	}

	res := SWbemServices{
		sWbemLocatorRaw: locatorIUnknown,
		sWbemLocator:    sWbemLocator,
	}
	return &res, nil
}

// InitializeSWbemServices will return a new SWbemServices object that can be used to query WMI.
//
// Deprecated: Use NewSWbemServices instead.
func InitializeSWbemServices(c *Client, connectServerArgs ...interface{}) (*SWbemServices, error) {
	s, err := NewSWbemServices()
	if err != nil {
		return nil, err
	}
	s.Decoder = c.Decoder
	return s, nil
}

// Close will clear and release all of the SWbemServices resources.
func (s *SWbemServices) Close() error {
	s.Lock()
	defer s.Unlock()
	if s.sWbemLocator == nil {
		return fmt.Errorf("SWbemServices is not Initialized")
	}
	s.sWbemLocatorRaw.Release()
	s.sWbemLocator = nil
	s.sWbemLocatorRaw = nil
	comshim.Done()
	return nil
}

// Query runs the WQL query using a SWbemServices instance and appends the values to dst.
func (s *SWbemServices) Query(query string, dst interface{}, connectServerArgs ...interface{}) (err error) {
	s.Lock()
	if s.sWbemLocator == nil {
		s.Unlock()
		return fmt.Errorf("SWbemServices has been closed")
	}
	s.Unlock()

	serv, err := s.ConnectServer(connectServerArgs...)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := serv.Close(); closeErr != nil {
			err = multierror.Append(err, closeErr)
		}
	}()
	return serv.Query(query, dst)
}
