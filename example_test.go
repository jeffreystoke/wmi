//+build windows

package wmi_test

import (
	"fmt"
	"log"

	"github.com/bi-zone/wmi"
)

// Structure with some fields from Win32_Process
// Ref: https://docs.microsoft.com/en-us/windows/win32/cimwin32prov/win32-process
type win32Process struct {
	PID       uint32 `wmi:"ProcessId"` // Field name differ from Win32_Process
	Name      string // Same name as in Win32_Process structure
	UserField int    `wmi:"-"` // Shouldn't affect WMI fields
}

func Example_enumerateRunningProcesses() {
	var dst []win32Process

	q := wmi.CreateQueryFrom(&dst, "Win32_Process", "")
	fmt.Println(q)

	if err := wmi.Query(q, &dst); err != nil {
		log.Fatal(err)
	}
	for _, v := range dst {
		fmt.Printf("%6d\t%s\n", v.PID, v.Name)
	}
}
