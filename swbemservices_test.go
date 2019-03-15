// +build windows

package wmi

import (
	"testing"
)

func TestWbemQuery(t *testing.T) {
	s, err := ConnectSWbemServices()
	if err != nil {
		t.Fatalf("InitializeSWbemServices: %s", err)
	}

	var dst []Win32_Process
	q := CreateQuery(&dst, "WHERE name='lsass.exe'")
	errQuery := s.Query(q, &dst)
	if errQuery != nil {
		t.Fatalf("Query1: %s", errQuery)
	}
	count := len(dst)
	if count < 1 {
		t.Fatal("Query1: no results found for lsass.exe")
	}
	//fmt.Printf("dst[0].ProcessID=%d\n", dst[0].ProcessId)

	q2 := CreateQuery(&dst, "WHERE name='svchost.exe'")
	errQuery = s.Query(q2, &dst)
	if errQuery != nil {
		t.Fatalf("Query2: %s", errQuery)
	}
	count = len(dst)
	if count < 1 {
		t.Fatal("Query2: no results found for svchost.exe")
	}
	//for index, item := range dst {
	//	fmt.Printf("dst[%d].ProcessID=%d\n", index, item.ProcessId)
	//}
	errClose := s.Close()
	if errClose != nil {
		t.Fatalf("Close: %s", errClose)
	}
}

func TestWbemQueryNamespace(t *testing.T) {
	s, err := NewSWbemServices()
	if err != nil {
		t.Fatalf("InitializeSWbemServices: %s", err)
	}
	var dst []MSFT_NetAdapter
	q := CreateQuery(&dst, "")
	errQuery := s.Query(q, &dst, nil, "root\\StandardCimv2")
	if errQuery != nil {
		t.Fatalf("Query: %s", errQuery)
	}
	count := len(dst)
	if count < 1 {
		t.Fatal("Query: no results found for MSFT_NetAdapter in root\\StandardCimv2")
	}
	errClose := s.Close()
	if errClose != nil {
		t.Fatalf("Close: %s", errClose)
	}
}

type MSFT_NetAdapter struct {
	Name              string
	InterfaceIndex    int
	DriverDescription string
}
