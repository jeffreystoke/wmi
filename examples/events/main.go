package main

// In the example we are going to track some events happen on WMI subscriptions.
// This is a good way to show tricky cases in WMI results decoding.

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"github.com/bi-zone/go-ole"
	"github.com/jeffreystoke/wmi"
)

// Notifications source in "root\subscription".
// Important thing here is that we can get an event of 3 different types.
var query = `
SELECT * FROM __InstanceOperationEvent
WITHIN 5
WHERE
	 	TargetInstance ISA '__EventFilter'
	OR 	TargetInstance ISA '__FilterToConsumerBinding'
`

// wmiEvent has a straightforward implementation. The only non-usual thing here
// is access to the system property `Path_.Class`.
type wmiEvent struct {
	TimeStamp uint64 `wmi:"TIME_CREATED"`
	System    struct {
		Class string
	} `wmi:"Path_"`
	Instance instance `wmi:"TargetInstance"`
}

// `TargetInstance` property in `__InstanceOperationEvent` can contain one of
// 2 different classes (cos os our query). To handle this in statically typed
// language we could either use an interface or add all of them and fill the
// only one.
//
// Lets create a field for every result type + `.Class` field to select the
// right one.
type instance struct {
	Class      string
	CreatorSID string

	EventFilter  *eventFilter        `json:",omitempty"`
	EventBinding *eventFilterBinding `json:",omitempty"`
}

// UnmarshalOLE extracts system properties of the resulting object and then
// unmarshalls the result into the proper `instance` field.
func (i *instance) UnmarshalOLE(d wmi.Decoder, src *ole.IDispatch) error {
	// Here is a temp object for the fields common for both classes.
	var commonProps struct {
		System struct {
			Class string
		} `wmi:"Path_"`
		CreatorSID []byte
	}
	if err := d.Unmarshal(src, &commonProps); err != nil {
		return err
	}

	sid, err := unmarshalSID(commonProps.CreatorSID)
	if err != nil {
		return err
	}
	i.Class = commonProps.System.Class
	i.CreatorSID = sid

	// And here we unmarshal the right class based on the `class` string from
	// the object system property.
	switch i.Class {
	case "__EventFilter":
		i.EventFilter = &eventFilter{}
		return d.Unmarshal(src, i.EventFilter)
	case "__FilterToConsumerBinding":
		i.EventBinding = &eventFilterBinding{}
		return d.Unmarshal(src, i.EventBinding)
	}
	return fmt.Errorf("unknown target class %q", i.Class)
}

// Golang-core mad skillz.
// If you know a better way to unmarshal []byte SID - please open a PR.
func unmarshalSID(sid []byte) (string, error) {
	p := unsafe.Pointer(&sid[0])
	s := (*syscall.SID)(p)
	return s.String()
}

// eventFilter is a simple struct with common fields.
type eventFilter struct {
	Name           string
	EventNamespace string
	Query          string
	QueryLanguage  string
}

// eventFilterBinding has 2 reference fields, which is a bit more tricky.
type eventFilterBinding struct {
	Consumer eventConsumer `wmi:",ref"`
	Filter   eventFilter   `wmi:",ref"`
}

// eventConsumer is never returned as is - it is always some descendant, so
// the best thing we could do - extract a Type name.
type eventConsumer struct {
	Type string `wmi:"-"`
}

func (e *eventConsumer) UnmarshalOLE(d wmi.Decoder, src *ole.IDispatch) error {
	var systemProps struct {
		Path struct {
			Class string
		} `wmi:"Path_"`
	}
	if err := d.Unmarshal(src, &systemProps); err != nil {
		return err
	}
	e.Type = systemProps.Path.Class
	return nil
}

// To produce an event you could use a powershell script, e.g.
//		#Creating a new event filter
//		$ServiceFilter = ([wmiclass]"\\.\root\subscription:__EventFilter").CreateInstance()
//
//		# Set the properties of the instance
//		$ServiceFilter.QueryLanguage = 'WQL'
//		$ServiceFilter.Query = "select * from __instanceModificationEvent within 5 where targetInstance isa 'win32_Service'"
//		$ServiceFilter.Name = "ServiceFilter"
//		$ServiceFilter.EventNamespace = 'root\cimv2'
//
//		# Sets the instance in the namespace
//		$FilterResult = $ServiceFilter.Put()
//		$ServiceFilterObj = $FilterResult.Path

func main() {
	events := make(chan wmiEvent)
	q, err := wmi.NewNotificationQuery(events, query)
	if err != nil {
		log.Fatalf("Failed to create NotificationQuery; %s", err)
	}

	// Set namespace.
	q.SetConnectServerArgs(nil, `root\subscription`)

	// Set exit hook
	sigs := make(chan os.Signal, 1)
	done := make(chan error, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		done <- q.StartNotifications()
	}()

	log.Println("Listening for events")
	for {
		select {
		case ev := <-events:
			data, err := json.MarshalIndent(ev, "", "  ")
			if err != nil {
				log.Printf("[ERR] Failed to marshal event; %s", err)
			} else {
				log.Println(string(data))
			}
		case sig := <-sigs:
			log.Printf("Got system signal %s; stopping", sig)
			q.Stop()
			return
		case err := <-done: // Query will never stop here w/o error.
			log.Printf("[ERR] Got StartNotifications error; %s", err)
			return
		}
	}
}
