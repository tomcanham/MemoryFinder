package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("Kernel32.dll")

var winOpenProcess = kernel32.NewProc("OpenProcess")
var winGetSystemInfo = kernel32.NewProc("GetSystemInfo")
var winVirtualQueryEx = kernel32.NewProc("VirtualQueryEx")
var winReadProcessMemory = kernel32.NewProc("ReadProcessMemory")
var winCloseHandle = kernel32.NewProc("CloseHandle")

type SYSTEM_INFO struct {
	dwOemId                     uint32
	dwPageSize                  uint32
	lpMinimumApplicationAddress uintptr
	lpMaximumApplicationAddress uintptr
	dwActiveProcessorMask       uintptr
	dwNumberOfProcessors        uint32
	dwProcessorType             uint32
	dwAllocationGranularity     uint32
	wProcessorLevel             uint16
	wProcessorRevision          uint16
}

type AllocationProtect uint32

func (ap AllocationProtect) String() string {
	var s []string

	if ap&0x10 != 0 {
		s = append(s, "PAGE_EXECUTE")
	}

	if ap&0x20 != 0 {
		s = append(s, "PAGE_EXECUTE_READ")
	}

	if ap&0x40 != 0 {
		s = append(s, "PAGE_EXECUTE_READWRITE")
	}

	if ap&0x80 != 0 {
		s = append(s, "PAGE_EXECUTE_WRITECOPY")
	}

	if ap&0x01 != 0 {
		s = append(s, "PAGE_NOACCESS")
	}

	if ap&0x02 != 0 {
		s = append(s, "PAGE_READONLY")
	}

	if ap&0x04 != 0 {
		s = append(s, "PAGE_READWRITE")
	}

	if ap&0x08 != 0 {
		s = append(s, "PAGE_WRITECOPY")
	}

	if ap&0x40000000 != 0 {
		s = append(s, "PAGE_TARGETS_NO_UPDATE")
	}

	if ap&0x100 != 0 {
		s = append(s, "PAGE_GUARD")
	}

	if ap&0x200 != 0 {
		s = append(s, "PAGE_NOCACHE")
	}

	if ap&0x400 != 0 {
		s = append(s, "PAGE_WRITECOMBINE")
	}

	return strings.Join(s, " | ")
}

type MemoryState uint32

func (ms MemoryState) String() string {
	var s []string

	if ms&0x1000 != 0 {
		s = append(s, "MEM_COMMIT")
	}

	if ms&0x10000 != 0 {
		s = append(s, "MEM_FREE")
	}

	if ms&0x2000 != 0 {
		s = append(s, "MEM_RESERVE")
	}

	return strings.Join(s, " | ")
}

type MemoryType uint32

func (mt MemoryType) String() string {
	var s []string

	if mt&0x1000000 != 0 {
		s = append(s, "MEM_IMAGE")
	}

	if mt&0x40000 != 0 {
		s = append(s, "MEM_MAPPED")
	}

	if mt&0x20000 != 0 {
		s = append(s, "MEM_PRIVATE")
	}

	return strings.Join(s, " | ")
}

type MEMORY_BASIC_INFORMATION struct {
	BaseAddress       uintptr
	AllocationBase    uintptr
	AllocationProtect AllocationProtect
	RegionSize        uintptr
	State             MemoryState
	Protect           AllocationProtect
	Type              MemoryType
}

func GetSystemInfo() SYSTEM_INFO {
	var si SYSTEM_INFO

	winGetSystemInfo.Call(uintptr(unsafe.Pointer(&si)))
	return si
}

func GetMemoryBasicInfo(handle, address uintptr) (MEMORY_BASIC_INFORMATION, bool) {
	var mbi MEMORY_BASIC_INFORMATION

	bytes, _, _ := winVirtualQueryEx.Call(uintptr(handle), address, uintptr(unsafe.Pointer(&mbi)), unsafe.Sizeof(mbi))
	return mbi, bytes == unsafe.Sizeof(mbi)
}

func (mbi *MEMORY_BASIC_INFORMATION) IsInteresting() bool {
	return mbi.Protect&0x04 != 0 && // PAGE_READWRITE
		mbi.Type&0x20000 != 0 && // MEM_PRIVATE
		mbi.State&0x1000 != 0 && // MEM_COMMIT
		mbi.Protect&0x100 == 0 // !PAGE_GUARD
}

func CloseHandle(handle uintptr) {
	result, _, lastError := winCloseHandle.Call(handle)

	if result == 0 {
		panic(fmt.Sprintf("CloseHandle 0x%x failed: %v", handle, lastError))
	}
}

var PROCESS_VM_READ uint32 = 0x0010
var PROCESS_QUERY_INFORMATION uint32 = 0x0400
var PROCESS_VM_WRITE uint32 = 0x0020
var PROCESS_VM_OPERATION uint32 = 0x0008
var dwDesiredAccess uint32 = PROCESS_VM_READ | PROCESS_QUERY_INFORMATION | PROCESS_VM_WRITE | PROCESS_VM_OPERATION

func OpenProcessWithDebug(processID uint32) uintptr {
	handle, _, lastError := winOpenProcess.Call(uintptr(dwDesiredAccess), uintptr(0), uintptr(processID))
	if handle == 0 {
		panic(fmt.Sprintf("Failed to open process id %d, error: %q\n", processID, lastError))
	}

	return handle
}

func (mbi *MEMORY_BASIC_INFORMATION) Read(hProcess, offset uintptr) []byte {
	b := make([]byte, mbi.RegionSize)
	var bytesRead uintptr

	success, _, lastError := winReadProcessMemory.Call(uintptr(hProcess), mbi.BaseAddress+uintptr(offset), uintptr(unsafe.Pointer(&b[0])), mbi.RegionSize, uintptr(unsafe.Pointer(&bytesRead)))
	if success != 1 {
		fmt.Printf("ReadProcessMemory failed: %v\n", lastError)
		fmt.Printf("%d bytes read\n", bytesRead)
	}

	return b
}

type MemoryFinder struct {
	processId  uint32
	systemInfo SYSTEM_INFO
	handle     uintptr
	mbis       []MEMORY_BASIC_INFORMATION
	target     uint32
	wg         sync.WaitGroup
	mut        sync.Mutex
	results    []uintptr
}

func (mf *MemoryFinder) Init(processId uint32) {
	mf.processId = processId
	mf.systemInfo = GetSystemInfo()
	mf.handle = OpenProcessWithDebug(processId)
	mf.WalkMemory()
}

func (mf *MemoryFinder) Close() {
	CloseHandle(mf.handle)
}

func (mf *MemoryFinder) WalkMemory() {
	var mbis []MEMORY_BASIC_INFORMATION

	cur := mf.systemInfo.lpMinimumApplicationAddress

	for {
		mbi, ok := GetMemoryBasicInfo(mf.handle, uintptr(cur))
		if !ok {
			break
		}

		if mbi.IsInteresting() {
			mbis = append(mbis, mbi)
		}

		cur = mbi.BaseAddress + mbi.RegionSize
	}

	mf.mbis = mbis
}

func (mf *MemoryFinder) AddResult(result uintptr) {
	mf.mut.Lock()
	defer mf.mut.Unlock()

	mf.results = append(mf.results, result)
}

func (mf *MemoryFinder) FindHelper(mbi MEMORY_BASIC_INFORMATION, search uint32) {
	defer mf.wg.Done()
	buffer := mbi.Read(mf.handle, 0)
	searchBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(searchBytes, search)

	index := bytes.Index(buffer, searchBytes)
	window := buffer[0:]
	for ; index > -1; index = bytes.Index(window, searchBytes) {
		if index > -1 {
			fmt.Printf("[0x%x] index: %v\n", mbi.BaseAddress, cap(buffer)-cap(window)+index)
			window = window[index+1:]
		} else {
			break
		}
	}
}

func (mf *MemoryFinder) Find(search uint32) {
	for _, mbi := range mf.mbis {
		mf.wg.Add(1)
		go mf.FindHelper(mbi, search)
	}

	mf.wg.Wait()
}

func findStringWorker(mbi MEMORY_BASIC_INFORMATION, handle uint32, wg *sync.WaitGroup) {
	defer wg.Done()
}

func findUint32(processID uint32, search uint32) {
	var mf MemoryFinder
	mf.Init(processID)
	defer mf.Close()

	mf.Find(search)
	fmt.Printf("%d total mbi's\n", len(mf.mbis))
}
