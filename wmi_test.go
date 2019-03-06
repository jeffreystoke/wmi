// +build windows

package wmi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime/debug"
	"testing"
)

func TestQuery(t *testing.T) {
	var dst []Win32_Process
	q := CreateQuery(&dst, "")
	err := Query(q, &dst)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFieldMismatch(t *testing.T) {
	type s struct {
		Name        string
		HandleCount uint32
		Blah        uint32
	}
	var dst []s
	err := Query("SELECT Name, HandleCount FROM Win32_Process", &dst)
	if err == nil || err.Error() != `wmi: cannot load field "Blah" into a "uint32": no such struct field` {
		t.Error("Expected err field mismatch")
	}
}

func TestStrings(t *testing.T) {
	printed := false
	f := func() {
		var dst []Win32_Process
		zeros := 0
		q := CreateQuery(&dst, "")
		for i := 0; i < 5; i++ {
			err := Query(q, &dst)
			if err != nil {
				t.Fatal(err, q)
			}
			for _, d := range dst {
				v := reflect.ValueOf(d)
				for j := 0; j < v.NumField(); j++ {
					f := v.Field(j)
					if f.Kind() != reflect.String {
						continue
					}
					s := f.Interface().(string)
					if len(s) > 0 && s[0] == '\u0000' {
						zeros++
						if !printed {
							printed = true
							j, _ := json.MarshalIndent(&d, "", "  ")
							t.Log("Example with \\u0000:\n", string(j))
						}
					}
				}
			}
			fmt.Println("iter", i, "zeros:", zeros)
		}
		if zeros > 0 {
			t.Error("> 0 zeros")
		}
	}

	fmt.Println("Disabling GC")
	debug.SetGCPercent(-1)
	f()
	fmt.Println("Enabling GC")
	debug.SetGCPercent(100)
	f()
}

func TestNamespace(t *testing.T) {
	var dst []Win32_Process
	q := CreateQuery(&dst, "")
	err := QueryNamespace(q, &dst, `root\CIMV2`)
	if err != nil {
		t.Fatal(err)
	}
	dst = nil
	err = QueryNamespace(q, &dst, `broken\nothing`)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateQuery(t *testing.T) {
	type TestStruct struct {
		Name  string
		Count int
	}
	var dst []TestStruct
	output := "SELECT Name, Count FROM TestStruct WHERE Count > 2"
	tests := []interface{}{
		&dst,
		dst,
		TestStruct{},
		&TestStruct{},
	}
	for i, test := range tests {
		if o := CreateQuery(test, "WHERE Count > 2"); o != output {
			t.Error("bad output on", i, o)
		}
	}
	if CreateQuery(3, "") != "" {
		t.Error("expected empty string")
	}
}
