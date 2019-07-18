// +build windows

/*
Package wmi provides a WQL interface for WMI on Windows.

Example code to print names of running processes:
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
*/
package wmi
