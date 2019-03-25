# wmi
[![GoDoc](https://godoc.org/github.com/bi-zone/wmi?status.svg)](https://godoc.org/github.com/bi-zone/wmi/)
[![Go Report Card](https://goreportcard.com/badge/github.com/bi-zone/wmi)](https://goreportcard.com/report/github.com/bi-zone/wmi)

Package wmi provides a WQL interface to Windows WMI.

Note: It interfaces with WMI on the local machine, therefore it only runs on Windows.

## Example
 Print names of the currently running processes
 ```golang
package main

import (
	"fmt"
	"log"

	"github.com/bi-zone/wmi"
)

// When we use `wmi.CreateQuery` the name of the struct should match querying
// WMI class name.
type Win32_Process struct {
	PID       uint32 `wmi:"ProcessId"`
	Name      string
	UserField int `wmi:"-"`
}

func main() {
	var dst []Win32_Process

	q := wmi.CreateQuery(&dst, "")
	fmt.Println(q)

	if err := wmi.Query(q, &dst); err != nil {
		log.Fatal(err)
	}
	for _, v := range dst {
		fmt.Println(v.PID, v.Name)
	}
}
 ```
 
 A more sophisticated examples are located at in [`examples`](./examples) folder.

## Benchmarks
Using `DefaultClient`, `SWbemServices` or `SWbemServicesConnection` differ in a number
of setup calls doing to perform each query (from the most to the least).

Estimated overhead is shown below:
```
BenchmarkQuery_DefaultClient   5000  33529798 ns/op
BenchmarkQuery_SWbemServices   5000  32031199 ns/op
BenchmarkQuery_SWbemConnection 5000  30099403 ns/op
```

You could reproduce the results on your machine running:
```bash
go test -run=NONE -bench=Query -benchtime=120s
```