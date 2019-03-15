package wmi

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/hashicorp/go-multierror"
	"github.com/scjalliance/comshim"
)

var (
	// ErrAlreadyRunning is returned when NotificationQuery is already running.
	ErrAlreadyRunning = errors.New("already running")
)

const (
	wbemErrTimedOut            = 0x80043001
	defaultNotificationTimeout = time.Second
)

// NotificationQuery represents subscription to the WMI events.
// For more info see https://docs.microsoft.com/en-us/windows/desktop/wmisdk/swbemservices-execnotificationquery
type NotificationQuery struct {
	Decoder

	sync.Mutex
	query             string
	state             state
	doneCh            chan struct{}
	eventCh           interface{}
	connectServerArgs []interface{}
	queryTimeoutMs    int64
}

// NewNotificationQuery creates a NotificationQuery from the given WQL @query
// string. The method just creates the object and does no WMI calls, so all WMI
// errors (query syntax, connection, etc.) will be returned on query start.
//
// @eventCh should be a channel of structures or structure pointers. The
// structure type should satisfy limitations described in `Decoder.Unmarshal`.
//
// Returns error if @eventCh is not `chan T` nor `chan *T`.
func NewNotificationQuery(eventCh interface{}, query string) (*NotificationQuery, error) {
	if !isChannelTypeOK(eventCh) {
		return nil, errors.New("eventCh has incorrect type; should be `chan T` or `chan *T`")
	}
	q := NotificationQuery{
		state:   stateNotStarted,
		doneCh:  make(chan struct{}),
		eventCh: eventCh,
		query:   query,
	}
	q.SetNotificationTimeout(defaultNotificationTimeout)
	return &q, nil
}

// SetNotificationTimeout specifies a time query could send waiting for the next
// event at the worst case. Waiting for the next event locks notification thread
// so in other words @t specifies a time for notification thread to react to the
// `Stop()` command at the worst.
//
// Default NotificationTimeout is 1s. It could be safely changed after the query
// `Start()`.
//
// Setting it to negative Duration makes that interval infinite.
func (q *NotificationQuery) SetNotificationTimeout(t time.Duration) {
	q.Lock()
	defer q.Unlock()
	if t < 0 {
		q.queryTimeoutMs = -1
		return
	}
	q.queryTimeoutMs = int64(t / time.Microsecond)
}

// SetConnectServerArgs sets `SWbemLocator.ConnectServer` args. Args are
// directly passed to `ole` call and support most of primitive types.
// Should be called before query being started.
//
// Args reference: https://docs.microsoft.com/en-us/windows/desktop/wmisdk/swbemlocator-connectserver
// Passing details: https://github.com/go-ole/go-ole/blob/master/idispatch_windows.go#L60
func (q *NotificationQuery) SetConnectServerArgs(args ...interface{}) {
	q.connectServerArgs = args
}

// StartNotifications connects to the WMI service and starts receiving
// notifications generated by the query.
//
// Errors are usually happen on initialization phase (connect to WMI,
// query execution, first result unmarshalling) so you could assume that
// "it's either starts and going to give me notifications or fails fast enough".
func (q *NotificationQuery) StartNotifications() (err error) {
	q.Lock()
	switch q.state {
	case stateStarted:
		q.Unlock()
		return ErrAlreadyRunning
	case stateStopped:
		q.Unlock()
		return nil
	}
	q.state = stateStarted
	q.Unlock()

	//  Be aware of reflections and COM usage.
	defer func() {
		if r := recover(); r != nil {
			err = multierror.Append(err, fmt.Errorf("runtime panic; %v", err))
		}
	}()

	// Notify that we are going to use COM.
	comshim.Add(1)
	defer comshim.Done()

	// Connect to WMI service.
	service, err := createWMIConnection(q.connectServerArgs...)
	if err != nil {
		return fmt.Errorf("failed to connect WMI service; %s", err)
	}
	defer service.Release()

	// Subscribe to the events. ExecNotificationQuery call must have that flags
	// and no other.
	sWbemEventSource, err := oleutil.CallMethod(
		service,
		"ExecNotificationQuery",
		q.query,
		"WQL",
		0x00000010|0x00000020, // WBEM_FLAG_RETURN_IMMEDIATELY | WBEM_FLAG_FORWARD_ONLY
	)
	if err != nil {
		return fmt.Errorf("ExecNotificationQuery failed; %s", err)
	}
	eventSource := sWbemEventSource.ToIDispatch()
	defer eventSource.Release()

	reflectedDoneChan := reflect.ValueOf(q.doneCh)
	reflectedResChan := reflect.ValueOf(q.eventCh)
	eventType := reflectedResChan.Type().Elem()
	for {
		// If it is a time to stop somebody will listen on doneCh.
		select {
		case q.doneCh <- struct{}{}:
			return nil
		default:
		}

		// Or try to query new events waiting no longer than queryTimeoutMs.
		eventIUnknown, err := eventSource.CallMethod("NextEvent", q.queryTimeoutMs)
		if err != nil {
			if isTimeoutError(err) {
				continue
			}
			return fmt.Errorf("unexpected NextEvent error; %s", err)
		}
		event := eventIUnknown.ToIDispatch()

		// Unmarshal event.
		e := reflect.New(eventType)
		if err := q.Unmarshal(event, e.Interface()); err != nil {
			return fmt.Errorf("failed to unmarshal event; %s", err)
		}

		// Send to the user.
		sent := trySend(reflectedResChan, reflectedDoneChan, e.Elem())
		if !sent {
			return nil // Query stopped
		}
	}
}

// Stop stops the running query waiting until everything is released. It could
// take some time for query to receive a stop signal. See `SetNotificationTimeout`
// for more info.
func (q *NotificationQuery) Stop() {
	q.Lock()
	defer q.Unlock()
	if q.state == stateStarted {
		<-q.doneCh
	}
	q.state = stateStopped
}

func createWMIConnection(connectServerArgs ...interface{}) (wmi *ole.IDispatch, err error) {
	sWbemLocatorIUnknown, err := oleutil.CreateObject("WbemScripting.SWbemLocator")
	if err != nil {
		return nil, fmt.Errorf("failed to create SWbemLocator; %s", err)
	} else if sWbemLocatorIUnknown == nil {
		return nil, ErrNilCreateObject
	}
	defer sWbemLocatorIUnknown.Release()

	sWbemLocatorIDispatch, err := sWbemLocatorIUnknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return nil, fmt.Errorf("SWbemLocator.QueryInterface failed ; %s", err)
	}
	defer sWbemLocatorIDispatch.Release()

	serviceRaw, err := oleutil.CallMethod(sWbemLocatorIDispatch, "ConnectServer", connectServerArgs...)
	if err != nil {
		return nil, fmt.Errorf("SWbemLocator.ConnectServer failed; %s", err)
	}
	return serviceRaw.ToIDispatch(), nil
}

type state int

const (
	stateNotStarted state = iota
	stateStarted
	stateStopped
)

func isTimeoutError(err error) bool {
	oleErr, ok := err.(*ole.OleError)
	return ok && oleErr.Code() == wbemErrTimedOut
}

func isChannelTypeOK(eventCh interface{}) bool {
	chT := reflect.TypeOf(eventCh)
	if chT.Kind() != reflect.Chan {
		return false
	}
	elemT := chT.Elem()
	switch elemT.Kind() {
	case reflect.Struct:
		return true
	case reflect.Ptr:
		return elemT.Elem().Kind() == reflect.Struct
	}
	return false
}

// trySend does a send in select block like:
//     select {
//     case resCh <- resEl:
//         return true
//     case doneCh <- struct{}{}:
//         return false
//     }
func trySend(resCh, doneCh, resEl reflect.Value) (sendSuccessful bool) {
	resCase := reflect.SelectCase{
		Dir:  reflect.SelectSend,
		Chan: resCh,
		Send: resEl,
	}
	doneCase := reflect.SelectCase{
		Dir:  reflect.SelectSend,
		Chan: doneCh,
		Send: reflect.ValueOf(struct{}{}),
	}
	idx, _, _ := reflect.Select([]reflect.SelectCase{resCase, doneCase})
	return idx == 0
}
