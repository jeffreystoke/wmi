package wmi

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// Run using: `TEST_MEM=1 go test -run TestWbemMemory -timeout 60m`
func TestWbemMemory(t *testing.T) {
	if os.Getenv("TEST_MEM") == "" {
		t.Skip("Skipping TestWbemMemory; $TEST_MEM is not set")
	}
	s, err := InitializeSWbemServices(DefaultClient)
	if err != nil {
		t.Fatalf("InitializeSWbemServices: %s", err)
	}
	start := time.Now()
	limit := 500000
	fmt.Printf("Benchmark Iterations: %d (Memory should stabilize around 7MB after ~3000)\n", limit)
	var privateMB, allocMB, allocTotalMB float64
	for i := 0; i < limit; i++ {
		privateMB, allocMB, allocTotalMB = wbemGetMemoryUsageMB(s)
		if i%100 == 0 {
			privateMB, allocMB, allocTotalMB = wbemGetMemoryUsageMB(s)
			fmt.Printf("Time: %4ds  Count: %5d  Private Memory: %5.1fMB  MemStats.Alloc: %4.1fMB  MemStats.TotalAlloc: %5.1fMB\n", time.Now().Sub(start)/time.Second, i, privateMB, allocMB, allocTotalMB)
		}
	}
	errClose := s.Close()
	if errClose != nil {
		t.Fatalf("Close: %s", err)
	}
	fmt.Printf("Final Time: %4ds  Private Memory: %5.1fMB  MemStats.Alloc: %4.1fMB  MemStats.TotalAlloc: %5.1fMB\n", time.Now().Sub(start)/time.Second, privateMB, allocMB, allocTotalMB)
}

// Run using: `TEST_MEM=1 go test -run TestMemoryWMISimple -timeout 60m`
func TestMemoryWMISimple(t *testing.T) {
	if os.Getenv("TEST_MEM") == "" {
		t.Skip("Skipping TestMemoryWMISimple; $TEST_MEM is not set")
	}

	start := time.Now()
	limit := 500000
	fmt.Printf("Benchmark Iterations: %d (Memory should stabilize around 7MB after ~3000)\n", limit)
	var privateMB, allocMB, allocTotalMB float64
	//var dst []Win32_PerfRawData_PerfDisk_LogicalDisk
	//q := CreateQuery(&dst, "")
	for i := 0; i < limit; i++ {
		privateMB, allocMB, allocTotalMB = getMemoryUsageMB()
		if i%1000 == 0 {
			//privateMB, allocMB, allocTotalMB = getMemoryUsageMB()
			fmt.Printf("Time: %4ds  Count: %5d  Private Memory: %5.1fMB  MemStats.Alloc: %4.1fMB  MemStats.TotalAlloc: %5.1fMB\n", time.Now().Sub(start)/time.Second, i, privateMB, allocMB, allocTotalMB)
		}
		//Query(q, &dst)
	}
	//privateMB, allocMB, allocTotalMB = getMemoryUsageMB()
	fmt.Printf("Final Time: %4ds  Private Memory: %5.1fMB  MemStats.Alloc: %4.1fMB  MemStats.TotalAlloc: %5.1fMB\n", time.Now().Sub(start)/time.Second, privateMB, allocMB, allocTotalMB)
}

// Run using: `TEST_MEM=1 go test -run TestMemoryWMIConcurrent -timeout 60m`
func TestMemoryWMIConcurrent(t *testing.T) {
	if os.Getenv("TEST_MEM") == "" {
		t.Skip("Skipping TestMemoryWMIConcurrent; $TEST_MEM is not set")
	}

	start := time.Now()
	limit := 50000
	fmt.Println("Total Iterations:", limit)
	fmt.Println("No panics mean it succeeded. Other errors are OK. Memory should stabilize after ~1500 iterations.")
	runtime.GOMAXPROCS(2)
	wg := sync.WaitGroup{}
	wg.Add(2)
	go func() {
		for i := 0; i < limit; i++ {
			if i%500 == 0 {
				privateMB, allocMB, allocTotalMB := getMemoryUsageMB()
				fmt.Printf("Time: %4ds  Count: %4d  Private Memory: %5.1fMB  MemStats.Alloc: %4.1fMB  MemStats.TotalAlloc: %5.1fMB\n", time.Now().Sub(start)/time.Second, i, privateMB, allocMB, allocTotalMB)
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
		for i := 0; i > -limit; i-- {
			//if i%500 == 0 {
			//	fmt.Println(i)
			//}
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
	//privateMB, allocMB, allocTotalMB := getMemoryUsageMB()
	//fmt.Printf("Final Private Memory: %5.1fMB  MemStats.Alloc: %4.1fMB  MemStats.TotalAlloc: %5.1fMB\n", privateMB, allocMB, allocTotalMB)
}

var lockthread sync.Mutex
var refcount1 int32
var refcount2 int32

// Test function showing memory leak in unknown.QueryInterface call on Server2016/Windows10
func getRSS(url string, xmlhttp *ole.IDispatch, MinimalTest bool) (int, error) {

	// call using url,nil to see memory leak
	if xmlhttp == nil {
		//Initialize inside loop if not passed in from outer section
		lockthread.Lock()
		defer lockthread.Unlock()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
		if err != nil {
			oleCode := err.(*ole.OleError).Code()
			if oleCode != ole.S_OK && oleCode != S_FALSE {
				return 0, err
			}
		}
		defer ole.CoUninitialize()

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

	/* This just bloats the TotalAlloc and slows the test down. Doesn't effect Private Working Set
	for n := 0; n < length; n++ {
		itemRaw := oleutil.MustGetProperty(items, "item", n)
		item := itemRaw.ToIDispatch()
		title := oleutil.MustCallMethod(item, "selectSingleNode", "title").ToIDispatch()

		//fmt.Println(oleutil.MustGetProperty(title, "text").ToString())
		textRaw := oleutil.MustGetProperty(title, "text")
		textRaw.ToString()

		link := oleutil.MustCallMethod(item, "selectSingleNode", "link").ToIDispatch()
		//fmt.Println("  " + oleutil.MustGetProperty(link, "text").ToString())
		textRaw2 := oleutil.MustGetProperty(link, "text")
		textRaw2.ToString()

		textRaw2.Clear()
		link.Release()
		textRaw.Clear()
		title.Release()
		itemRaw.Clear()
	}
	*/
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
	fmt.Printf("Benchmark Iterations: %d (Memory should stabilize around 8MB to 12MB after ~2k full or 250k minimal)\n", limit)

	//On Server 2016 or Windows 10 changing leakMemory=true will cause it to leak ~1.5MB per 10000 calls to unknown.QueryInterface
	leakMemory := true

	////////////////////////////////////////
	//Start outer section
	var unknown *ole.IUnknown
	var xmlhttp *ole.IDispatch
	if !leakMemory {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
		if err != nil {
			oleCode := err.(*ole.OleError).Code()
			if oleCode != ole.S_OK && oleCode != S_FALSE {
				t.Fatal(err)
			}
		}
		defer ole.CoUninitialize()

		//fmt.Println("CreateObject Microsoft.XMLHTTP")
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
			privateMB, allocMB, allocTotalMB := getMemoryUsageMB()
			fmt.Printf("Time: %4ds  Count: %7d  Private Memory: %5.1fMB  MemStats.Alloc: %4.1fMB  MemStats.TotalAlloc: %5.1fMB  %7d/%7d\n", time.Now().Sub(start)/time.Second, i, privateMB, allocMB, allocTotalMB, refcount1, refcount2)
		}
		//This should use less than 10MB for 1 million iterations if xmlhttp was initialized above
		//On Server 2016 or Windows 10 changing leakMemory=true above will cause it to leak ~1.5MB per 10000 calls to unknown.QueryInterface
		count, err := getRSS(url, xmlhttp, true) //last argument is for Minimal test. Doesn't effect leak just overall allocations/time
		if err != nil {
			t.Fatal(err)
		}
		totalItems += uint64(count)
	}
	privateMB, allocMB, allocTotalMB := getMemoryUsageMB()
	fmt.Printf("Final totalItems: %d  Private Memory: %5.1fMB  MemStats.Alloc: %4.1fMB  MemStats.TotalAlloc: %5.1fMB\n", totalItems, privateMB, allocMB, allocTotalMB)
}

const MB = 1024 * 1024

var (
	mMemoryUsageMB      runtime.MemStats
	errGetMemoryUsageMB error
	dstGetMemoryUsageMB []Win32_PerfRawData_PerfProc_Process
	filterProcessID     = fmt.Sprintf("WHERE IDProcess = %d", os.Getpid())
	qGetMemoryUsageMB   = CreateQuery(&dstGetMemoryUsageMB, filterProcessID)
)

func getMemoryUsageMB() (float64, float64, float64) {
	runtime.ReadMemStats(&mMemoryUsageMB)
	//errGetMemoryUsageMB = nil //Query(qGetMemoryUsageMB, &dstGetMemoryUsageMB) float64(dstGetMemoryUsageMB[0].WorkingSetPrivate)
	errGetMemoryUsageMB = Query(qGetMemoryUsageMB, &dstGetMemoryUsageMB)
	if errGetMemoryUsageMB != nil {
		fmt.Println("ERROR GetMemoryUsage", errGetMemoryUsageMB)
		return 0, 0, 0
	}
	return float64(dstGetMemoryUsageMB[0].WorkingSetPrivate) / MB, float64(mMemoryUsageMB.Alloc) / MB, float64(mMemoryUsageMB.TotalAlloc) / MB
}

func wbemGetMemoryUsageMB(s *SWbemServices) (float64, float64, float64) {
	runtime.ReadMemStats(&mMemoryUsageMB)
	errGetMemoryUsageMB = s.Query(qGetMemoryUsageMB, &dstGetMemoryUsageMB)
	if errGetMemoryUsageMB != nil {
		fmt.Println("ERROR GetMemoryUsage", errGetMemoryUsageMB)
		return 0, 0, 0
	}
	return float64(dstGetMemoryUsageMB[0].WorkingSetPrivate) / MB, float64(mMemoryUsageMB.Alloc) / MB, float64(mMemoryUsageMB.TotalAlloc) / MB
}
