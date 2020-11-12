// +build windows

package wmi

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/bi-zone/go-ole"
	"github.com/bi-zone/go-ole/oleutil"
	"github.com/jeffreystoke/comshim"
)

const memReps = 50 * 1000

// Run using: `TEST_MEM=1 go test -run TestMemory_Services -timeout 60m`
func TestMemory_Services(t *testing.T) {
	if os.Getenv("TEST_MEM") == "" {
		t.Skip("Skipping TestMemory_Services; $TEST_MEM is not set")
	}
	s, err := NewSWbemServices()
	if err != nil {
		t.Fatalf("InitializeSWbemServices: %s", err)
	}

	start := time.Now()
	fmt.Printf("Benchmark Iterations: %d (Private Memory should stabilize around 7MB after ~3000)\n", memReps)

	var privateMB, allocMB, allocTotalMB float64
	for i := 0; i < memReps; i++ {
		privateMB, allocMB, allocTotalMB = wbemGetMemoryUsageMB(t, s)
		if i%100 == 0 {
			fmt.Printf("Time: %4ds  Count: %5d  ", time.Now().Sub(start)/time.Second, i)
			printlnMemUsage(privateMB, allocMB, allocTotalMB)
		}
	}

	errClose := s.Close()
	if errClose != nil {
		t.Fatalf("Close: %s", err)
	}

	fmt.Printf("Final Time: %4ds ", time.Now().Sub(start)/time.Second)
	printlnMemUsage(privateMB, allocMB, allocTotalMB)
}

// Run using: `TEST_MEM=1 go test -run TestMemory_WbemConnection -timeout 60m`
func TestMemory_WbemConnection(t *testing.T) {
	if os.Getenv("TEST_MEM") == "" {
		t.Skip("Skipping TestMemory_WbemConnection; $TEST_MEM is not set")
	}
	s, err := ConnectSWbemServices()
	if err != nil {
		t.Fatalf("InitializeSWbemServices: %s", err)
	}

	start := time.Now()
	fmt.Printf("Benchmark Iterations: %d (Private Memory should stabilize around 7MB after ~3000)\n", memReps)

	var privateMB, allocMB, allocTotalMB float64
	for i := 0; i < memReps; i++ {
		privateMB, allocMB, allocTotalMB = wbemConnGetMemoryUsageMB(t, s)
		if i%100 == 0 {
			fmt.Printf("Time: %4ds  Count: %5d  ", time.Now().Sub(start)/time.Second, i)
			printlnMemUsage(privateMB, allocMB, allocTotalMB)
		}
	}

	errClose := s.Close()
	if errClose != nil {
		t.Fatalf("Close: %s", err)
	}

	fmt.Printf("Final Time: %4ds ", time.Now().Sub(start)/time.Second)
	printlnMemUsage(privateMB, allocMB, allocTotalMB)
}

// Run using: `TEST_MEM=1 go test -run TestMemory_WMISimple -timeout 60m`
func TestMemory_WMISimple(t *testing.T) {
	if os.Getenv("TEST_MEM") == "" {
		t.Skip("Skipping TestMemory_WMISimple; $TEST_MEM is not set")
	}

	start := time.Now()
	fmt.Printf("Benchmark Iterations: %d (Private Memory should stabilize around 7MB after ~3000)\n", memReps)

	var privateMB, allocMB, allocTotalMB float64
	for i := 0; i < memReps; i++ {
		privateMB, allocMB, allocTotalMB = getMemoryUsageMB(t)
		if i%1000 == 0 {
			fmt.Printf("Time: %4ds  Count: %5d  ", time.Now().Sub(start)/time.Second, i)
			printlnMemUsage(privateMB, allocMB, allocTotalMB)
		}
	}

	fmt.Printf("Final Time: %4ds ", time.Now().Sub(start)/time.Second)
	printlnMemUsage(privateMB, allocMB, allocTotalMB)
}

// Run using: `TEST_MEM=1 go test -run TestMemory_WMIConcurrent -timeout 60m`
func TestMemory_WMIConcurrent(t *testing.T) {
	if os.Getenv("TEST_MEM") == "" {
		t.Skip("Skipping TestMemory_WMIConcurrent; $TEST_MEM is not set")
	}

	fmt.Println("Total Iterations:", memReps)
	fmt.Println("No panics mean it succeeded. Other errors are OK. Private Memory should stabilize after ~1500 iterations.")
	runtime.GOMAXPROCS(2)

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		for i := 0; i < memReps; i++ {
			if i%500 == 0 {
				privateMB, allocMB, allocTotalMB := getMemoryUsageMB(t)
				fmt.Printf("Time: %4ds  Count: %5d  ", time.Now().Sub(start)/time.Second, i)
				printlnMemUsage(privateMB, allocMB, allocTotalMB)
			}
			var dst []Win32_PerfRawData_PerfDisk_LogicalDisk
			q := CreateQuery(&dst, "")
			err := Query(q, &dst)
			if err != nil {
				fmt.Println("ERROR disk", err)
			}
		}
		wg.Done()
	}()
	go func() {
		for i := 0; i > -memReps; i-- {
			var dst []Win32_OperatingSystem
			q := CreateQuery(&dst, "")
			err := Query(q, &dst)
			if err != nil {
				fmt.Println("ERROR OS", err)
			}
		}
		wg.Done()
	}()
	wg.Wait()
	fmt.Printf("Final Time: %4ds\n", time.Now().Sub(start)/time.Second)
}

// In that test case we are going to ensure that large amount of timeout
// exceptions won't leak a memory (as it does in github.com/go-ole/go-ole@1.2.4
// and below).
//
// Run using: `TEST_MEM=1 go test -run TestMemory_OLEErrors -timeout 60m`
func TestMemory_OLEErrors(t *testing.T) {
	if os.Getenv("TEST_MEM") == "" {
		t.Skip("Skipping TestMemory_OLEErrors; $TEST_MEM is not set")
	}

	// Subscribe to some rare event. E.g. removal of the local drive.
	const query = "SELECT * FROM Win32_VolumeChangeEvent WHERE EventType=3"

	// We don't care about events, so listen to chan of nothing.
	events := make(chan struct{})
	q, err := NewNotificationQuery(events, query)
	if err != nil {
		t.Fatalf("Failed to create NotificationQuery; %s", err)
	}

	// Set some really small notification timeout so we will get a lot of timeouts.
	q.SetNotificationTimeout(time.Millisecond)

	// Start listening notifications. Blocks until stop.
	done := make(chan error, 1)
	go func() {
		done <- q.StartNotifications()
	}()

	go func() {
		t.Log("Listening for events")
		for range events { // Do nothing.
		}
	}()

	const sleep = 30 * time.Second
	const N = (50 * time.Minute) / sleep
	fmt.Printf("Benchmark Iterations: %d (Private Memory should stabilize around 7MB after ~5min)\n", N)

	start := time.Now()
	for i := 0; i < int(N); i++ {
		time.Sleep(sleep)

		privateMB, allocMB, allocTotalMB := getMemoryUsageMB(t)
		fmt.Printf("Time: %4ds  ", time.Now().Sub(start)/time.Second)
		printlnMemUsage(privateMB, allocMB, allocTotalMB)
	}

	q.Stop()
	close(events)
	if err := <-done; err != nil {
		t.Errorf("Error listening to notifications query: %s", err)
	}
}

// Run using: `TEST_MEM=1 go test -run TestMemory_Dereference -timeout 60m`
func TestMemory_Dereference(t *testing.T) {
	if os.Getenv("TEST_MEM") == "" {
		t.Skip("Skipping TestMemory_Dereference; $TEST_MEM is not set")
	}

	type netAdapter struct {
		Adapter struct {
			MACAddress string
		} `wmi:"Element,ref"`
		Settings struct {
			IPEnabled   bool
			DHCPEnabled bool
		} `wmi:"Setting,ref"`
	}
	var dumbRes []netAdapter
	testQueryMem(t, &dumbRes,
		CreateQueryFrom(netAdapter{}, "Win32_NetworkAdapterSetting ", ""))
}

// Run using: `TEST_MEM=1 go test -run TestMemory_OLEArrays -timeout 60m`
func TestMemory_OLEArrays(t *testing.T) {
	if os.Getenv("TEST_MEM") == "" {
		t.Skip("Skipping TestMemory_OLEArrays; $TEST_MEM is not set")
	}

	// Actually we could use any class and use in system fields, but Win32_BIOS
	// already has arrays inside, so it's a free COMBO!
	type miniBIOS struct {
		Derivation_         []string // System-property. Available for any class.
		BIOSVersion         []string
		BiosCharacteristics []uint16
	}
	var dumbRes []miniBIOS
	testQueryMem(t, &dumbRes,
		// Write a query manually cos we shouldn't specify a system field in WQL string.
		"SELECT * FROM Win32_BIOS")
}

// testQueryMem tests memory usage performing repeating WMI queries for a long time.
func testQueryMem(t *testing.T, dst interface{}, query string) {
	s, err := ConnectSWbemServices()
	if err != nil {
		t.Fatalf("ConnectSWbemServices: %s", err)
	}

	start := time.Now()
	fmt.Printf("Benchmark Iterations: %d (Private Memory should stabilize around 7MB after ~3000)\n", memReps)

	var privateMB, allocMB, allocTotalMB float64
	for i := 0; i < memReps; i++ {
		if err := s.Query(query, dst); err != nil {
			t.Fatalf("Failed to perform query %q; %s", query, err)
		}

		if i%100 == 0 {
			privateMB, allocMB, allocTotalMB = wbemConnGetMemoryUsageMB(t, s)
			fmt.Printf("Time: %4ds  Count: %5d  ", time.Now().Sub(start)/time.Second, i)
			printlnMemUsage(privateMB, allocMB, allocTotalMB)
		}
	}

	if errClose := s.Close(); errClose != nil {
		t.Fatalf("Close: %s", errClose)
	}

	fmt.Printf("Final Time: %4ds ", time.Now().Sub(start)/time.Second)
	printlnMemUsage(privateMB, allocMB, allocTotalMB)
}

var refcount1 int32
var refcount2 int32

// Test function showing memory leak in unknown.QueryInterface call on Server2016/Windows10
func getRSS(url string, xmlhttp *ole.IDispatch, MinimalTest bool) (int, error) {

	// call using url,nil to see memory leak
	if xmlhttp == nil {
		//Initialize inside loop if not passed in from outer section
		comshim.Add(1)
		defer comshim.Done()

		//fmt.Println("CreateObject Microsoft.XMLHTTP")
		unknown, err := oleutil.CreateObject("Microsoft.XMLHTTP")
		if err != nil {
			return 0, err
		}
		defer func() { refcount1 += xmlhttp.Release() }()

		//Memory leak occurs here
		xmlhttp, err = unknown.QueryInterface(ole.IID_IDispatch)
		if err != nil {
			return 0, err
		}
		defer func() { refcount2 += xmlhttp.Release() }()
		//Nothing below this really matters. Can be removed if you want a tighter loop
	}

	//fmt.Printf("Download %s\n", url)
	openRaw, err := oleutil.CallMethod(xmlhttp, "open", "GET", url, false)
	if err != nil {
		return 0, err
	}
	defer openRaw.Clear()

	if MinimalTest {
		return 1, nil
	}

	//Initiate http request
	sendRaw, err := oleutil.CallMethod(xmlhttp, "send", nil)
	if err != nil {
		return 0, err
	}
	defer sendRaw.Clear()
	state := -1 // https://developer.mozilla.org/en-US/docs/Web/API/XMLHttpRequest/readyState
	for state != 4 {
		time.Sleep(5 * time.Millisecond)
		stateRaw := oleutil.MustGetProperty(xmlhttp, "readyState")
		state = int(stateRaw.Val)
		stateRaw.Clear()
	}

	responseXMLRaw := oleutil.MustGetProperty(xmlhttp, "responseXml")
	responseXML := responseXMLRaw.ToIDispatch()
	defer responseXMLRaw.Clear()
	itemsRaw := oleutil.MustCallMethod(responseXML, "selectNodes", "/rdf:RDF/item")
	items := itemsRaw.ToIDispatch()
	defer itemsRaw.Clear()
	lengthRaw := oleutil.MustGetProperty(items, "length")
	defer lengthRaw.Clear()
	length := int(lengthRaw.Val)

	return length, nil
}

// Testing go-ole/oleutil
// Run using: `TEST_MEM=1 go test -run TestMemoryOLE -timeout 60m`
// Code from https://github.com/go-ole/go-ole/blob/master/example/msxml/rssreader.go
func TestMemoryOLE(t *testing.T) {
	if os.Getenv("TEST_MEM") == "" {
		t.Skip("Skipping TestMemoryOLE; $TEST_MEM is not set")
	}

	defer func() {
		if r := recover(); r != nil {
			t.Error(r)
		}
	}()

	start := time.Now()
	limit := 50000000
	url := "http://localhost/slashdot.xml" //http://rss.slashdot.org/Slashdot/slashdot"
	fmt.Printf("Benchmark Iterations: %d (Private Memory should stabilize around 8MB to 12MB after ~2k full or 250k minimal)\n", limit)

	//On Server 2016 or Windows 10 changing leakMemory=true will cause it to leak ~1.5MB per 10000 calls to unknown.QueryInterface
	leakMemory := true

	////////////////////////////////////////
	//Start outer section
	var unknown *ole.IUnknown
	var xmlhttp *ole.IDispatch
	if !leakMemory {
		comshim.Add(1)
		defer comshim.Done()

		//fmt.Println("CreateObject Microsoft.XMLHTTP")
		var err error
		unknown, err = oleutil.CreateObject("Microsoft.XMLHTTP")
		if err != nil {
			t.Fatal(err)
		}
		defer unknown.Release()

		//Memory leak starts here
		xmlhttp, err = unknown.QueryInterface(ole.IID_IDispatch)
		if err != nil {
			t.Fatal(err)
		}
		defer xmlhttp.Release()
	}
	//End outer section
	////////////////////////////////////////

	totalItems := uint64(0)
	for i := 0; i < limit; i++ {
		if i%2000 == 0 {
			privateMB, allocMB, allocTotalMB := getMemoryUsageMB(t)
			fmt.Printf("Time: %4ds  Count: %7d  ", time.Now().Sub(start)/time.Second, i)
			printlnMemUsage(privateMB, allocMB, allocTotalMB)
		}
		//This should use less than 10MB for 1 million iterations if xmlhttp was initialized above
		//On Server 2016 or Windows 10 changing leakMemory=true above will cause it to leak ~1.5MB per 10000 calls to unknown.QueryInterface
		count, err := getRSS(url, xmlhttp, true) //last argument is for Minimal test. Doesn't effect leak just overall allocations/time
		if err != nil {
			t.Fatal(err)
		}
		totalItems += uint64(count)
	}
	privateMB, allocMB, allocTotalMB := getMemoryUsageMB(t)
	fmt.Printf("Final totalItems: %d ", totalItems)
	printlnMemUsage(privateMB, allocMB, allocTotalMB)

}

const MB = 1024 * 1024

var (
	mMemoryUsageMB      runtime.MemStats
	errGetMemoryUsageMB error
	dstGetMemoryUsageMB []Win32_PerfRawData_PerfProc_Process
	filterProcessID     = fmt.Sprintf("WHERE IDProcess = %d", os.Getpid())
	qGetMemoryUsageMB   = CreateQuery(&dstGetMemoryUsageMB, filterProcessID)
)

func getMemoryUsageMB(t *testing.T) (float64, float64, float64) {
	runtime.ReadMemStats(&mMemoryUsageMB)
	errGetMemoryUsageMB = Query(qGetMemoryUsageMB, &dstGetMemoryUsageMB)
	if errGetMemoryUsageMB != nil {
		t.Fatalf("ERROR GetMemoryUsage; %s", errGetMemoryUsageMB)
	}
	return float64(dstGetMemoryUsageMB[0].WorkingSetPrivate) / MB, float64(mMemoryUsageMB.Alloc) / MB, float64(mMemoryUsageMB.TotalAlloc) / MB
}

func wbemGetMemoryUsageMB(t *testing.T, s *SWbemServices) (float64, float64, float64) {
	runtime.ReadMemStats(&mMemoryUsageMB)
	errGetMemoryUsageMB = s.Query(qGetMemoryUsageMB, &dstGetMemoryUsageMB)
	if errGetMemoryUsageMB != nil {
		t.Fatalf("ERROR GetMemoryUsage; %s", errGetMemoryUsageMB)
	}
	return float64(dstGetMemoryUsageMB[0].WorkingSetPrivate) / MB, float64(mMemoryUsageMB.Alloc) / MB, float64(mMemoryUsageMB.TotalAlloc) / MB
}

func wbemConnGetMemoryUsageMB(t *testing.T, s *SWbemServicesConnection) (float64, float64, float64) {
	runtime.ReadMemStats(&mMemoryUsageMB)
	errGetMemoryUsageMB = s.Query(qGetMemoryUsageMB, &dstGetMemoryUsageMB)
	if errGetMemoryUsageMB != nil {
		t.Fatalf("ERROR GetMemoryUsage; %s", errGetMemoryUsageMB)
	}
	return float64(dstGetMemoryUsageMB[0].WorkingSetPrivate) / MB, float64(mMemoryUsageMB.Alloc) / MB, float64(mMemoryUsageMB.TotalAlloc) / MB
}

func printlnMemUsage(privateMB, allocMB, allocTotalMB float64) {
	fmt.Printf("Private Memory: %5.1fMB  MemStats.Alloc: %4.1fMB  MemStats.TotalAlloc: %5.1fMB\n", privateMB, allocMB, allocTotalMB)
}
