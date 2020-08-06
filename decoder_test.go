// +build windows

package wmi

import (
	"errors"
	"fmt"
	"os/user"
	"testing"
	"time"

	"github.com/bi-zone/go-ole"
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
	} else if !(system.CreationDate.After(someOldDate) && system.CreationDate.Before(testStart)) {
		t.Errorf("Unexoected System process creation date; %s", system.CreationDate)
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
	err := client.Query(`SELECT * FROM Win32_Process WHERE ProcessId = 4`, &processes)
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

// A few Win32_Process fields with tags.
type taggedMiniProcess struct {
	Name      string // Same as real.
	PID       uint32 `wmi:"ProcessId"` // Modified name.
	UserField string `wmi:"-"`         // Any non-property field.
}

func TestDecoder_Unmarshal_Tags(t *testing.T) {
	// Query with all fields having receiver with not all.
	var processes []taggedMiniProcess
	err := Query(`	SELECT * FROM Win32_Process WHERE ProcessId = 4`, &processes)
	if err != nil {
		t.Fatalf("Failed to query running processes; %s", err)
	}
	// Get System process (always exists with PID=4)
	if len(processes) != 1 {
		t.Fatalf("Failed to find System (PID=4) process in running processes")
	}
	system := processes[0]

	// Check the fields.
	if system.PID != 4 {
		t.Fatalf("Unexpected System process PID; got %v, expected %v", system.PID, 4)
	}
	if system.Name != "System" {
		t.Errorf("Unexpected System process name; got %v, expected %v", system.Name, "System")
	}
	if system.UserField != "" {
		t.Errorf("Spoiled field marked to skip; content %q", system.UserField)
	}
}

// Very self-sufficient process struct that is able to handle unmarshalling of
// itself.
type selfMadeProcess struct {
	HexPID          string // Just because.
	CoolProcessName string // Plus some beautification.
}

// Example of `wmi.Unmarshaler` interface implementation.
func (p *selfMadeProcess) UnmarshalOLE(d Decoder, src *ole.IDispatch) (err error) {
	var dto struct {
		Name      string
		ProcessId uint32
	}
	if err := d.Unmarshal(src, &dto); err != nil {
		return err
	}
	p.HexPID = fmt.Sprintf("0x%x", dto.ProcessId)
	p.CoolProcessName = fmt.Sprintf("-=%s=-", dto.Name)
	return nil
}

type dumbUnmarshaller struct{}

func (dumbUnmarshaller) UnmarshalOLE(d Decoder, src *ole.IDispatch) error {
	return errors.New("always fail")
}

func TestDecoder_Unmarshal_Unmarshaler(t *testing.T) {
	// Query with all fields having receiver with not all.
	var processes []selfMadeProcess
	err := Query(`	SELECT * FROM Win32_Process WHERE ProcessId = 4`, &processes)
	if err != nil {
		t.Fatalf("Failed to query running processes; %s", err)
	}
	// Get System process (always exists with PID=4)
	if len(processes) != 1 {
		t.Fatalf("Failed to find System (PID=4) process in running processes")
	}
	system := processes[0]

	// Check the fields.
	if system.HexPID != "0x4" {
		t.Fatalf("Unexpected System process PID; got %q, expected %q", system.HexPID, "0x4")
	}
	if system.CoolProcessName != "-=System=-" {
		t.Errorf("Unexpected System process name; got %q, expected %q", system.CoolProcessName, "-=System=-")
	}

	// Check that interface error passed to the output.
	var failer []dumbUnmarshaller
	err = Query(`SELECT * FROM Win32_Process WHERE ProcessId = 4`, &failer)
	if err == nil {
		t.Fatal("Failed to proxy Unmarshaler error to the caller")
	}
}

// Win32_BIOS
type miniBIOS struct {
	Version             string
	BIOSVersion         []string
	BiosCharacteristics []uint16
}

func TestDecoder_Unmarshal_Slices(t *testing.T) {
	var bioses []miniBIOS
	query := "SELECT * FROM Win32_BIOS"
	err := Query(query, &bioses)
	if err != nil || len(bioses) < 1 {
		t.Fatalf("Failed to query Win32_BIOS; %v", err)
	}

	t.Logf("The following test can fail on some tricky installs cos of unpredictable WMI results, ")
	t.Logf("so please check test result manually before start to panic.")

	t.Logf("Results for query %q:", query)
	for _, v := range bioses {
		t.Logf("%#v", v)
	}

	bios := bioses[0]
	if len(bios.Version) < 1 {
		t.Fatalf("Empty BIOS version string")
	}
	if len(bios.BIOSVersion) < 1 {
		t.Errorf("Empty BIOS versions list")
	} else if len(bios.BIOSVersion[0]) < 1 {
		t.Errorf("Empty string in BIOS version liss; %v", bios.Version)
	}
	if len(bios.BiosCharacteristics) < 1 {
		t.Errorf("Empty BIOS characteristics list")
	} else if bios.BiosCharacteristics[0] == 0 {
		t.Errorf("Unexpected BiosCharacteristics %v", bios.BiosCharacteristics[0])
	}
}

// Win32_UserProfile
// https://msdn.microsoft.com/en-us/library/ee886409(v=vs.85).aspx
type userProfile struct {
	SID string
	// https://docs.microsoft.com/en-us/previous-versions/windows/desktop/usm/win32-folderredirectionhealth
	Desktop struct {
		OfflineFileNameFolderGUID string
	}
	AppDataRoaming *struct {
		OfflineFileNameFolderGUID string
	}
}

func TestDecoder_Unmarshal_EmbeddedStructures(t *testing.T) {
	// Fetch current user.
	u, err := user.Current()
	if err != nil {
		t.Fatalf("Failed to query current user")
	}

	// Extract user profile of the current user.
	query := fmt.Sprintf(`SELECT * FROM Win32_UserProfile WHERE SID = '%s'`, u.Uid)
	t.Logf(query)

	var users []userProfile
	if err := Query(query, &users); err != nil {
		t.Fatalf("Failed to query Win32_UserProfile; %v", err)
	}
	if len(users) < 1 {
		t.Fatalf("No profiles found")
	}
	t.Logf("Results:\n%#v", users)

	profile := users[0]
	if profile.SID != u.Uid {
		t.Errorf("Queried unexpected user; got %v, expected %v", profile.SID, u.Uid)
	}
	// AFAIK should always be non-empty string.
	if profile.AppDataRoaming.OfflineFileNameFolderGUID == "" {
		t.Errorf("Queried empty OfflineFileNameFolderGUID for AppDataRoaming")
	}
	if profile.Desktop.OfflineFileNameFolderGUID == "" {
		t.Errorf("Queried empty OfflineFileNameFolderGUID for Desktop")
	}
}

// Win32_LoggedOnUser
// https://docs.microsoft.com/en-us/windows/desktop/cimwin32prov/win32-loggedonuser
type loggedUser struct {
	Session struct {
		LogonId string
	} `wmi:"Dependent,ref"`
	Account struct {
		SID string
	} `wmi:"Antecedent,ref"`
}

func TestDecoder_Unmarshal_References(t *testing.T) {
	// Fetch current user.
	u, err := user.Current()
	if err != nil {
		t.Fatalf("Failed to query current user")
	}

	// Extract user profile of the current user.
	var users []loggedUser
	if err := Query(`SELECT * FROM Win32_LoggedOnUser`, &users); err != nil {
		// Sometimes we can't fetch full info about all users but for the current test it's
		// pretty ok.
		t.Logf("Got errors querying Win32_LoggedOnUser; %v", err)
	}
	if len(users) < 1 {
		t.Fatalf("No logged users found")
	}

	var current *loggedUser
	for i, lu := range users {
		if lu.Account.SID == u.Uid {
			current = &users[i]
		}
	}

	if current == nil {
		t.Fatalf("Failed to find current user (SID=%q) session in %+v", u.Uid, users)
	}
	if current.Session.LogonId == "" {
		t.Errorf("Unexpected LogonID of current user; got %q", current.Session.LogonId)
	}
}
