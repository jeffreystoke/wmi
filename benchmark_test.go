package wmi

import "testing"

//Run all benchmarks (should run for at least 60s to get a stable number):
//go test -run=NONE -bench=. -benchtime=120s

// All Query benchmarks:
// go test -run=NONE -bench=Query -benchtime=120s

// go test -run=NONE -bench=Query_DefaultClient -benchtime=120s
func BenchmarkQuery_DefaultClient(b *testing.B) {
	var dst []Win32_OperatingSystem
	q := CreateQuery(&dst, "")
	for n := 0; n < b.N; n++ {
		errQuery := Query(q, &dst)
		if errQuery != nil {
			b.Fatalf("Query%d: %s", n, errQuery)
		}
		count := len(dst)
		if count < 1 {
			b.Fatalf("Query%d: no results found for Win32_OperatingSystem", n)
		}
	}
}

// go test -run=NONE -bench=Query_SWbemServices -benchtime=120s
func BenchmarkQuery_SWbemServices(b *testing.B) {
	s, err := NewSWbemServices()
	if err != nil {
		b.Fatalf("InitializeSWbemServices: %s", err)
	}

	var dst []Win32_OperatingSystem
	q := CreateQuery(&dst, "")
	for n := 0; n < b.N; n++ {
		errQuery := s.Query(q, &dst)
		if errQuery != nil {
			b.Fatalf("Query%d: %s", n, errQuery)
		}
		count := len(dst)
		if count < 1 {
			b.Fatalf("Query%d: no results found for Win32_OperatingSystem", n)
		}
	}

	errClose := s.Close()
	if errClose != nil {
		b.Fatalf("Close: %s", errClose)
	}
}

// go test -run=NONE -bench=Query_SWbemConnection -benchtime=120s
func BenchmarkQuery_SWbemConnection(b *testing.B) {
	s, err := ConnectSWbemServices() // Note here
	if err != nil {
		b.Fatalf("InitializeSWbemServices: %s", err)
	}

	var dst []Win32_OperatingSystem
	q := CreateQuery(&dst, "")
	for n := 0; n < b.N; n++ {
		errQuery := s.Query(q, &dst)
		if errQuery != nil {
			b.Fatalf("Query%d: %s", n, errQuery)
		}
		count := len(dst)
		if count < 1 {
			b.Fatalf("Query%d: no results found for Win32_OperatingSystem", n)
		}
	}

	errClose := s.Close()
	if errClose != nil {
		b.Fatalf("Close: %s", errClose)
	}
}
