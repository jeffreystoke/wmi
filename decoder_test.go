// +build windows

package wmi

import (
	"testing"
	"time"
)

var (
	testStart   = time.Now()
	someOldDate = time.Unix(0, 0) // Hope ur PC doesn't run since 1970s.
)

func TestDecoder_Unmarshal(t *testing.T) {
	// Query some processes with all available fields.
	var processes []Win32_Process
	err := Query("SELECT * FROM Win32_Process WHERE ProcessId = 4", &processes)
	if err != nil {
		t.Fatalf("Failed to query running processes; %s", err)
	}

	// Get System process (always exists with PID=4)
	if len(processes) != 1 {
		t.Fatalf("Failed to find System (PID=4) process in running processes")
	}
	system := processes[0]

	// Check some fields with different types.
	// Uint32:
	if system.ProcessId != 4 {
		t.Fatalf("Unexpected System process PID; got %v, expected %v", system.ProcessId, 4)
	}
	// String:
	if system.Name != "System" {
		t.Errorf("Unexpected System process name; got %v, expected %v", system.Name, "System")
	}
	// String pointer:
	if system.Description == nil {
		t.Errorf("Failed to fecth Win32_Process.Description")
	} else if *system.Description != "System" {
		t.Errorf("Got unexpected System process Description")
	}
	// Time pointer:
	if system.CreationDate == nil {
		t.Errorf("Failed to fetch Win32_Process.CreationDate")
	} else {
		if !(system.CreationDate.After(someOldDate) && system.CreationDate.Before(testStart)) {
			t.Errorf("Unexoected System process creation date; %s", system.CreationDate)
		}
	}
	// uint64:
	if system.KernelModeTime == 0 {
		t.Errorf("Failed to fetch Win32_Process.KernelModeTime")
	}
	// Always nil pointer:
	if system.TerminationDate != nil {
		t.Errorf("Unexpected termination date for System process; got %v", system.TerminationDate)
	}
}

// A bit modified version of Win32_Process.
type miniProcess struct {
	Name           string
	ProcessId      int       // cast uint32 -> int
	CreationDate   time.Time // non-pointer receiver
	KernelModeTime uint      // uint64 -> uint
}

func TestDecoder_Unmarshal_ModifiedFields(t *testing.T) {
	// Query some processes with all existing fields.
	var processes []miniProcess
	err := Query(`
		SELECT Name, ProcessId, CreationDate, KernelModeTime 
		FROM Win32_Process 
		WHERE ProcessId = 4`, &processes)
	if err != nil {
		t.Fatalf("Failed to query running processes; %s", err)
	}

	// Get System process (always exists with PID=4)
	if len(processes) != 1 {
		t.Fatalf("Failed to find System (PID=4) process in running processes")
	}
	system := processes[0]

	// Check the fields.
	if system.ProcessId != 4 {
		t.Fatalf("Unexpected System process PID; got %v, expected %v", system.ProcessId, 4)
	}
	if system.Name != "System" {
		t.Errorf("Unexpected System process name; got %v, expected %v", system.Name, "System")
	}
	someOldDate := time.Unix(0, 0) // Hope ur PC doesn't run since 1970s.
	if !(system.CreationDate.After(someOldDate) && system.CreationDate.Before(testStart)) {
		t.Errorf("Unexoected System process creation date; %s", system.CreationDate)
	}
	if system.KernelModeTime == 0 {
		t.Errorf("Failed to fetch Win32_Process.KernelModeTime")
	}
}

func TestDecoder_Unmarshal_OmitUnneeded(t *testing.T) {
	// Create test client with modified config to not mess other tests.
	var client Client
	client.Decoder.AllowMissingFields = true
	// Query with all fields having receiver with not all.
	var processes []miniProcess
	err := client.Query(`	SELECT * FROM Win32_Process WHERE ProcessId = 4`, &processes)
	if err != nil {
		t.Fatalf("Failed to query running processes; %s", err)
	}
	// Get System process (always exists with PID=4)
	if len(processes) != 1 {
		t.Fatalf("Failed to find System (PID=4) process in running processes")
	}
	// Check that anything were queried.
	empty := miniProcess{}
	if processes[0] == empty {
		t.Errorf("Failed to fill anything in process; got %+v", processes[0])
	}
}
