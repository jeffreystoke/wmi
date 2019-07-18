// +build windows

package wmi

import (
	"os/user"
	"strings"
	"testing"
)

// Just a smoke test of SWbemServicesConnection API. More detailed ones has
// been done at the upper levels of abstraction.
func TestSWbemServicesConnection(t *testing.T) {
	s, err := ConnectSWbemServices()
	if err != nil {
		t.Fatalf("ConnectSWbemServices: %s", err)
	}

	var dst []Win32_OperatingSystem
	q := CreateQuery(&dst, "")
	errQuery := s.Query(q, &dst)
	if errQuery != nil {
		t.Fatalf("Query: %s", errQuery)
	}

	count := len(dst)
	if count < 1 {
		t.Fatalf("Query: no results found for Win32_OperatingSystem")
	}

	errClose := s.Close()
	if errClose != nil {
		t.Fatalf("Close: %s", errClose)
	}
}

type userAccount struct {
	SID    string
	Name   string
	Domain string
}

func TestSWbemServicesConnection_Get(t *testing.T) {
	s, err := ConnectSWbemServices()
	if err != nil {
		t.Fatalf("InitializeSWbemServices: %s", err)
	}

	// https://docs.microsoft.com/en-us/windows/desktop/cimwin32prov/win32-loggedonuser
	var loggedUsers []struct {
		AccountRef string `wmi:"Antecedent"`
	}
	if err := s.Query("SELECT * from Win32_LoggedOnUser", &loggedUsers); err != nil {
		t.Fatalf("Failed to query Win32_LoggedOnUser; %s", err)
	}

	osUser, err := user.Current()
	if err != nil {
		t.Fatalf("Failed to query current u; %s", err)
	}

	// Try to find current user account by its SID.
	var currentUserAccount userAccount
	for _, u := range loggedUsers {
		var account userAccount
		if err := s.Get(u.AccountRef, &account); err != nil {
			t.Errorf("Failed to get Win32_Account using ref %q; %s", u, err)
			continue
		}

		if account.SID == osUser.Uid {
			currentUserAccount = account
			break
		}
	}

	t.Logf("Expected user session; %+v", currentUserAccount)

	// And now check domain/username to ensure we got the right user.
	parts := strings.SplitN(osUser.Username, `\`, 2)
	osUserDomain, osUserName := parts[0], parts[1]
	if currentUserAccount.Name != osUserName {
		t.Errorf("Got unexpected user Name; got %q, expected %q", currentUserAccount.Name, osUserName)
	}
	if currentUserAccount.Domain != osUserDomain {
		t.Errorf("Got unexpected user Domain; got %q, expected %q", currentUserAccount.Domain, osUserDomain)
	}
}
