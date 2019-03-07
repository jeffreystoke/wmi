package wmi

import "testing"

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
