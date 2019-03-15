package wmi

import "testing"

// Just a smoke test of SWbemServicesConnection API. More detailed ones has
// been done at the upper levels of abstraction.
func TestSWbemServicesConnection(t *testing.T) {
	s, err := ConnectSWbemServices() // Note here
	if err != nil {
		t.Fatalf("InitializeSWbemServices: %s", err)
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
