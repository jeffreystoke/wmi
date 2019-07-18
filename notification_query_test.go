// +build windows

package wmi

import (
	"sync"
	"testing"
	"time"
)

func TestNewNotificationQuery(t *testing.T) {
	type T struct{} // Just struct
	cases := []struct {
		ch         interface{}
		shouldFail bool
	}{
		{make(chan T), false},
		{make(chan *T), false},
		{make(chan []T), true},
		{T{}, true},
		{&T{}, true},
		{make([]T, 0), true},
		{make([]*T, 0), true},
		{make(map[interface{}]T, 0), true},
		{make(map[interface{}]*T, 0), true},
	}
	for _, test := range cases {
		_, err := NewNotificationQuery(test.ch, "any")
		if test.shouldFail && err == nil {
			t.Errorf("Successfully created NotificationQuery with eventCh of type %T", test.ch)
		} else if !test.shouldFail && err != nil {
			t.Errorf("Failed to create NotificationQuery with eventCh of type %T", test.ch)
		}
	}
}

func TestNotificationQuery(t *testing.T) {
	type event struct {
		Created  uint64 `wmi:"TIME_CREATED"`
		Instance struct {
			Hour  uint32
			Day   uint32
			Month uint32
			Year  uint32
		} `wmi:"TargetInstance"`
	}

	resultCh := make(chan event)
	queryString := `SELECT * FROM __InstanceModificationEvent WHERE TargetInstance ISA 'Win32_LocalTime'`
	query, err := NewNotificationQuery(resultCh, queryString)
	if err != nil {
		t.Fatalf("Failed to create NotificationQuery; %s", err)
	}
	query.SetNotificationTimeout(100 * time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		if err := query.StartNotifications(); err != nil {
			t.Errorf("Notification query error; %s", err)
		}
		wg.Done()
	}()

	// Get the event.
	e := <-resultCh
	now := time.Now() // Notice time of event receive.

	// Stop the query and confirm routine is dead.
	query.Stop()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Errorf("Failed to stop query in 5x NotificationTimeout's")
	}

	// Check the event.
	if e.Created == 0 {
		t.Errorf("Got unexpected TIME_CREATED value; got %d expected non nil", e.Created)
	}
	if e.Instance.Hour != uint32(now.Hour()) {
		t.Errorf("Got unexpected Hour value; got %d expected %d", e.Instance.Hour, now.Hour())
	}
	if e.Instance.Day != uint32(now.Day()) {
		t.Errorf("Got unexpected Day value; got %d expected %d", e.Instance.Day, now.Day())
	}
	if e.Instance.Month != uint32(now.Month()) {
		t.Errorf("Got unexpected Month value; got %d expected %d", e.Instance.Month, now.Month())
	}
	if e.Instance.Year != uint32(now.Year()) {
		t.Errorf("Got unexpected Year value; got %d expected %d", e.Instance.Year, now.Year())
	}
}

func TestNotificationQuery_StartStop(t *testing.T) {
	type event struct {
		Created uint64 `wmi:"TIME_CREATED"`
	}

	resultCh := make(chan event)
	queryString := `SELECT * FROM __InstanceModificationEvent WHERE TargetInstance ISA 'Win32_LocalTime'`
	query, err := NewNotificationQuery(resultCh, queryString)
	if err != nil {
		t.Fatalf("Failed to create NotificationQuery; %s", err)
	}
	query.SetNotificationTimeout(100 * time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		if err := query.StartNotifications(); err != nil {
			t.Errorf("Notification query error; %s", err)
		}
		wg.Done()
	}()

	// Do not get the event!
	// Stop the query and confirm routine is dead.
	query.Stop()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Errorf("Failed to stop query in 5x NotificationTimeout's")
	}
}

func TestNotificationQuery_StopWithNoEvents(t *testing.T) {
	type event struct {
		Created uint64 `wmi:"TIME_CREATED"`
	}

	// Create a query that will never receive an event.
	resultCh := make(chan event)
	queryString := `
SELECT * FROM __InstanceModificationEvent
WHERE TargetInstance ISA 'Win32_LocalTime' AND TargetInstance.Hour = 25` // Should never happen.

	query, err := NewNotificationQuery(resultCh, queryString)
	if err != nil {
		t.Fatalf("Failed to create NotificationQuery; %s", err)
	}
	query.SetNotificationTimeout(100 * time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		if err := query.StartNotifications(); err != nil {
			t.Errorf("Notification query error; %s", err)
		}
		wg.Done()
	}()

	// We can't get an event, but emulate some tries.
	select {
	case e := <-resultCh:
		t.Errorf("OMFG! Got timer event with Hour == 25; %+v", e)
	case <-time.After(500 * time.Millisecond):
		// Ok. As intended.
	}

	// Stop the query and confirm routine is dead.
	query.Stop()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Errorf("Failed to stop query in 5x NotificationTimeout's")
	}
}
