package main

import (
	"fmt"
	"time"
	"unicode/utf16"
	"unsafe"
	"sync"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazyDLL("kernel32.dll")
	procReadDirectoryChangesW = kernel32.NewProc("ReadDirectoryChangesW")
	procReadFileEx            = kernel32.NewProc("ReadFileEx")
	procWriteFileEx           = kernel32.NewProc("WriteFileEx")
	procCreateFile            = kernel32.NewProc("CreateFileW")
	procCloseHandle           = kernel32.NewProc("CloseHandle")
	procWaitForSingleObject   = kernel32.NewProc("WaitForSingleObject")
	procGetNamedPipeClientPID  = kernel32.NewProc("GetNamedPipeClientProcessId")
	procGetNamedPipeServerPID  = kernel32.NewProc("GetNamedPipeServerProcessId")
	procGetNamedPipeHandleState = kernel32.NewProc("GetNamedPipeHandleStateW")
)

type namedPipeHandleStateStruct struct {
	success bool
    state uint32
    curInstances  uint32
	maxCollectionCount uint32
	collectDataTimeout uint32
	userName string
}

func getHandleOwner(handle windows.Handle, result *string, wg *sync.WaitGroup) {
	defer wg.Done()

	// Get the security descriptor
	sd, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return
	}

	// Get the owner SID from the security descriptor
	ownerSid, _, err := sd.Owner()
	if err != nil {
		return
	}

	// Convert SID to a readable username
	var nameSize, domainSize uint32
	var sidType uint32

	// First call to get buffer sizes
	windows.LookupAccountSid(nil, ownerSid, nil, &nameSize, nil, &domainSize, &sidType)

	// Allocate buffers
	nameBuffer := make([]uint16, nameSize)
	domainBuffer := make([]uint16, domainSize)

	// Second call to retrieve actual username and domain
	err = windows.LookupAccountSid(nil, ownerSid, &nameBuffer[0], &nameSize, &domainBuffer[0], &domainSize, &sidType)
	if err != nil {
		return
	}

	// Convert Windows UTF-16 to Go strings
	owner := fmt.Sprintf("[%s\\%s] ", windows.UTF16ToString(domainBuffer), windows.UTF16ToString(nameBuffer))
	*result = owner
	return 
}

func timeFormat(givenTime time.Time) string {
	return fmt.Sprintf(
		"%02d:%02d:%02d",
		givenTime.Hour(),
		givenTime.Minute(),
		givenTime.Second(),
	)
}

func waitForExitInput(isexit chan bool) {
	for {
		var char rune
		_, err := fmt.Scanf("%c", &char)
		if err == nil {
			_, err := fmt.Scanf("%c", &char)
			fmt.Printf("[*] Keyboard input received, press again to stop program\n")
			if err == nil {
				fmt.Printf("[*] Keyboard input received, exiting\n")
				isexit <- true
				return
			}
		}
	}
}

func readFromHandle(handle windows.Handle, overlapped bool) ([]byte, error) {
	var bufferSize uint32 = 1024
	var bufferContentSize uint32
	var data []byte
	overlapped_param := new(windows.Overlapped)
	for {
		buffer := make([]byte, bufferSize)
		if overlapped {
			err := windows.ReadFile(handle, buffer, &bufferContentSize, overlapped_param)
			if err != nil && err != windows.ERROR_IO_PENDING {
				return data, err
			}
		} else {
			err := windows.ReadFile(handle, buffer, &bufferContentSize, nil)
			if err != nil {
				return data, err
			}
		}
		data = append(data, buffer[:bufferContentSize]...)
		if bufferContentSize < bufferSize {
			break
		}
	}

	return data, nil
}

func writeToHandle(handle windows.Handle, data []byte, overlapped bool) error {
	defer windows.FlushFileBuffers(handle)
	var dataLen = len(data)
	var totalWritten = 0
	overlapped_param := new(windows.Overlapped)
	for totalWritten < dataLen {
		var chunkWritten uint32
		if overlapped {
			err := windows.WriteFile(handle, data[totalWritten:], &chunkWritten, overlapped_param)
			if err != nil && err != windows.ERROR_IO_PENDING {
				return err
			}
		} else {
			err := windows.WriteFile(handle, data[totalWritten:], &chunkWritten, nil)
			if err != nil {
				return err
			}
		}
		totalWritten += int(chunkWritten)
	}
	return nil
}

func bestFileHandle(fileName string) (windows.Handle, bool, bool, bool, bool, string) {
	rwAccess := false
	readAccess := false
	writeAccess := false
	controlAccess := false
	displayAccess := "  "

	// TRY RW
	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(fileName),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)

	if err == nil {
		readAccess = true
		writeAccess = true
		rwAccess = true
		controlAccess = true
		displayAccess = "RW"
		return handle, readAccess, writeAccess, rwAccess, controlAccess, displayAccess
	}

	// TRY READ
	handle, err = windows.CreateFile(
		windows.StringToUTF16Ptr(fileName),
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)

	if err == nil {
		readAccess = true
		displayAccess = "R-"
		return handle, readAccess, writeAccess, rwAccess, controlAccess, displayAccess
	}

	// TRY WRITE
	handle, err = windows.CreateFile(
		windows.StringToUTF16Ptr(fileName),
		windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)

	if err == nil {
		writeAccess = true
		displayAccess = "-W"
		return handle, readAccess, writeAccess, rwAccess, controlAccess, displayAccess
	}

	// TRY READ_CONTROL
	handle, err = windows.CreateFile(
		windows.StringToUTF16Ptr(fileName),
		windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)

	if err == nil {
		displayAccess = "--"
		controlAccess = true
	}

	return handle, readAccess, writeAccess, rwAccess, controlAccess, displayAccess
}

func tryFilePermissions(path string, permissions uint32, success chan bool) {

	// Convert the path to UTF16 format
	pPath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		success <- false
		return
	}

	handle, err := windows.CreateFile(pPath,
		permissions,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0)

	// We don't need the handle anymore, let's close it now without blocking function
	go windows.CloseHandle(handle)

	if err == nil {
		success <- true
		return
	}
	success <- false
}

func getFileOwner(filePath string, ownerChan chan string) {
	// Open the file handle with READ_CONTROL (only need metadata access)
	handle, err := windows.CreateFile(
		windows.StringToUTF16Ptr(filePath),
		windows.READ_CONTROL,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)

	// Close the handle after function return
	defer windows.CloseHandle(handle)

	if err != nil {
		ownerChan <- ""
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var owner string
	go getHandleOwner(handle, &owner, &wg)
	wg.Wait()
	ownerChan <- owner

	return
}

// checkFileAccess checks if the current user has read and write permissions to the given file or directory.
func checkFileAccess(path string) (bool, bool, string) {
	readSuccess := make(chan bool)
	writeSuccess := make(chan bool)

	go tryFilePermissions(path, windows.GENERIC_READ, readSuccess)
	go tryFilePermissions(path, windows.GENERIC_WRITE, writeSuccess)

	readAccess := <-readSuccess
	writeAccess := <-writeSuccess

	displayAccess := ""
	if readAccess {
		displayAccess += "R"
	} else {
		displayAccess += "-"
	}
	if writeAccess {
		displayAccess += "W"
	} else {
		displayAccess += "-"
	}

	return readAccess, writeAccess, displayAccess
}

// read ptr with given length
func utf16ToString(ptr *uint16, length uint32) string {
	if ptr == nil || length == 0 {
		return ""
	}

	// Convert bytes to uint16 array (each UTF-16 char is 2 bytes)
	utf16Chars := (*[1 << 20]uint16)(unsafe.Pointer(ptr))[: length/2 : length/2]
	return string(utf16.Decode(utf16Chars))
}

func GetNamedPipeClientPID(handle windows.Handle, result *uint32, wg *sync.WaitGroup){
	defer wg.Done()
	var buffer uint32
	ret, _, err := procGetNamedPipeClientPID.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&buffer)),
	)
	if ret == 0 {
		buffer = 0
	}
	if debug {
		fmt.Printf("[DEBUG] GetNamedPipeClientPID ret:%d err:%v result:%d\n", ret, err, buffer)
	}
	*result = buffer
}

func GetNamedPipeServerPID(handle windows.Handle, result *uint32, wg *sync.WaitGroup){
	defer wg.Done()
	var buffer uint32
	ret, _, err := procGetNamedPipeServerPID.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&buffer)),
	)
	if ret == 0 {
		buffer = 0
	}
	if debug {
		fmt.Printf("[DEBUG] GetNamedPipeServerPID ret:%d err:%v result:%d\n", ret, err, buffer)
	}
	*result = buffer
}

func GetNamedPipeHandleState(handle windows.Handle, result *namedPipeHandleStateStruct, wg *sync.WaitGroup){
	defer wg.Done()
	// var b1, b2, b3, b4 uint32
	var b1, b2 uint32
	// b5 := make([]uint16, 256) 

	ret, _, err := procGetNamedPipeHandleState.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&b1)),
		uintptr(unsafe.Pointer(&b2)),
		0,
		0,
		0,
		0,
	)
	if debug {
		fmt.Printf("[DEBUG] GetNamedPipeHandleState ret:%d err:%v\n", ret, err)
	}
	if ret != 0 {
		result.success = true
		result.state = b1
		result.curInstances = b2
		// result.maxCollectionCount = b3
		// result.collectDataTimeout = b4
		// result.userName = windows.UTF16ToString(b5)
	}
}