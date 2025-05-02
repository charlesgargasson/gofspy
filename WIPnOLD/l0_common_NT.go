package main

import (
	"fmt"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modNtdll                         = windows.NewLazySystemDLL("ntdll.dll")
	procNtQueryDirectoryFile         = modNtdll.NewProc("NtQueryDirectoryFile")
	procNtOpenFile                   = modNtdll.NewProc("NtOpenFile")
	procNtQueryInformationFile       = modNtdll.NewProc("NtQueryInformationFile")
	modAdvapi32                      = windows.NewLazySystemDLL("advapi32.dll")
	procInitializeSecurityDescriptor = modAdvapi32.NewProc("InitializeSecurityDescriptor")

	modkernel32       = windows.NewLazyDLL("kernel32.dll")
	procFormatMessage = modkernel32.NewProc("FormatMessageW")
)

type FILE_PIPE_INFORMATION_STRUCT struct {
	ReadMode            uint32
	CompletionMode      uint32
	CurrentInstances    uint32
	MaximumInstances    uint32
	InboundQuota        uint32
	ReadDataAvailable   uint32
	OutboundQuota       uint32
	WriteQuotaAvailable uint32
	PipeState           uint32
	PipeEnd             uint32
}

type OBJECT_ATTRIBUTES struct {
	Length                   uint32
	RootDirectory            windows.Handle
	ObjectName               *unicodeString
	Attributes               uint32
	SecurityDescriptor       uintptr
	SecurityQualityOfService uintptr
}

type SecurityQualityOfService struct {
	Length              uint32
	ImpersonationLevel  uint32
	ContextTrackingMode byte
	EffectiveOnly       byte
	Padding             [2]byte // Ensure proper alignment to match C struct layout
}

type SECURITY_DESCRIPTOR struct {
	Revision byte
	Sbz1     byte
	Control  uint16
	Owner    uintptr
	Group    uintptr
	Sacl     uintptr
	Dacl     uintptr
}

type unicodeString struct {
	Length        uint16
	MaximumLength uint16
	Buffer        *uint16
}

type IO_STATUS_BLOCK struct {
	Status      uint32
	Information uintptr
}

const (
	IO_STATUS_BLOCK_SIZE = 8
)

const (
	FORMAT_MESSAGE_FROM_SYSTEM = 0x00001000
)

func InitializeSecurityDescriptor(sd uintptr, rev uint32) (err error) {
	r1, _, e1 := procInitializeSecurityDescriptor.Call(sd, uintptr(rev))
	if r1 == 0 {
		err = e1
	}
	return
}

func RtlInitUnicodeString(dest *unicodeString, src *uint16) {
	length := 0
	ptr := uintptr(unsafe.Pointer(src))
	for ; *(*uint16)(unsafe.Pointer(ptr)) != 0; ptr += 2 {
		length++
	}

	dest.Length = uint16(length * 2)     // Length in bytes
	dest.MaximumLength = dest.Length + 2 // Room for null terminator
	dest.Buffer = src
}

// Convert NTSTATUS to string using FormatMessage
func ntStatusToMessage(status uint32) string {
	var buffer [256]uint16
	size, _, _ := procFormatMessage.Call(
		FORMAT_MESSAGE_FROM_SYSTEM,
		0,
		uintptr(status),
		0,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		0,
	)
	if size == 0 {
		return fmt.Sprintf("Unknown NTSTATUS: 0x%X", status)
	}
	return windows.UTF16ToString(buffer[:])
}

// 0xC0000034 STATUS_OBJECT_NAME_NOT_FOUND
// 0xc0000033 (Object Name invalid)
// 0xC0000022 Access denied
// 0xC0000008 Invalid handle
// 0xC000000D Invalid parameter

func NtOpenFile(fileHandle *windows.Handle, desiredAccess uint32, objectAttributes *OBJECT_ATTRIBUTES, ioStatusBlock *IO_STATUS_BLOCK, shareAccess uint32, openOptions uint32) (uint32, error) {
	ret, _, err := procNtOpenFile.Call(
		uintptr(unsafe.Pointer(fileHandle)),
		uintptr(desiredAccess),
		uintptr(unsafe.Pointer(objectAttributes)),
		uintptr(unsafe.Pointer(ioStatusBlock)),
		uintptr(shareAccess),
		uintptr(openOptions),
	)
	if ret != 0 {
		return uint32(ret), err
	}
	return 0, nil
}

func openPipeSilently(pipeName string) {
	pipePath := strings.Replace(pipeName, `\\.\pipe\`, `\Device\NamedPipe\`, -1)
	// pipePath := pipeName
	var handle windows.Handle
	fmt.Printf("Attempting to open pipe: %s\n", pipePath)

	var desiredAccess uint32 = windows.GENERIC_READ | windows.SYNCHRONIZE //
	var shareAccess uint32 = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE
	var openOptions uint32 = windows.OPEN_EXISTING | windows.FILE_OPEN // | windows.FILE_SYNCHRONOUS_IO_NONALERT

	// ---------------------------

	var ioStatusBlock IO_STATUS_BLOCK

	// Prepare a security descriptor
	sd := SECURITY_DESCRIPTOR{} // SECURITY_DESCRIPTOR struct size
	if err := InitializeSecurityDescriptor(uintptr(unsafe.Pointer(&sd)), 1); err != nil {
		fmt.Printf("InitializeSecurityDescriptor failed: %v\n", err)
		return
	}

	// Set up object attributes with security descriptor and SQOS
	sqos := SecurityQualityOfService{
		Length:              uint32(unsafe.Sizeof(SecurityQualityOfService{})),
		ImpersonationLevel:  2, // SecurityImpersonation level
		ContextTrackingMode: 1, // TRUE for dynamic tracking
		EffectiveOnly:       1, // Restrict to effective rights only
	}

	// Prepare the OBJECT_ATTRIBUTES
	// var objAttr OBJECT_ATTRIBUTES
	// fileName := windows.StringToUTF16Ptr(pipePath)

	// Prepare OBJECT_ATTRIBUTES with proper UnicodeString
	var objAttr OBJECT_ATTRIBUTES
	var unicodePipeName unicodeString
	pipeNamePtr, _ := windows.UTF16PtrFromString(pipePath)
	RtlInitUnicodeString(&unicodePipeName, pipeNamePtr)

	objAttr.Length = uint32(unsafe.Sizeof(OBJECT_ATTRIBUTES{}))
	//objAttr.ObjectName = (*unicodeString)(unsafe.Pointer(fileName))
	objAttr.ObjectName = &unicodePipeName
	objAttr.Attributes = windows.OBJ_CASE_INSENSITIVE                 // OBJ_CASE_INSENSITIVE
	objAttr.SecurityDescriptor = uintptr(unsafe.Pointer(&sd))         //
	objAttr.SecurityQualityOfService = uintptr(unsafe.Pointer(&sqos)) //
	objAttr.RootDirectory = 0                                         // Set to 0 if not using a root directory

	// Call NtOpenFile
	ret, err := NtOpenFile(&handle, desiredAccess, &objAttr, &ioStatusBlock, shareAccess, openOptions)
	if err != nil {
		fmt.Printf("Error: %v,\nStatus: %x\nMessage: %s", err, ret, ntStatusToMessage(ret))
		return
	}
	defer windows.CloseHandle(handle)

	// ---------------------------

	return

	// ---  procNtQueryInformationFile
}

func checkPipeNT(pipeName string, isexit chan bool) {
	hijackHandle, err := createDuplexPipe(pipeName)
	windows.CloseHandle(hijackHandle)
	if err != nil {
		fmt.Printf("💧 %s 🔴 Can't Hijack (%v)\n", timeFormat(time.Now()), err)
	} else {
		fmt.Printf("💧 %s 🟢 Hijackable \n", timeFormat(time.Now()))
	}

	openPipeSilently(pipeName)
	//if err != nil {
	//	fmt.Printf("💧 %s 🔴 openPipeSilently (%v)\n", timeFormat(time.Now()), err)
	//}

	//handle, err := openPipeSilently(pipeName)
	//if err != nil {
	//	fmt.Printf("💧 %s 🔴 Can't open pipe (%v)\n", timeFormat(time.Now()), err)
	//	isexit <- true
	//	return
	//}
	//defer windows.CloseHandle(handle)
	//fmt.Printf("💧 %s 🟢 Opened pipe, reading infos ... \n", timeFormat(time.Now()))

	//pipeInfo, err := queryPipeInfo(handle)
	//if err != nil {
	//	fmt.Printf("💧 %s 🔴 Can't query infos (%v)\n", timeFormat(time.Now()), err)
	//	isexit <- true
	//	return
	//}

	//fmt.Printf("ReadMode %d \n", pipeInfo.ReadMode)
	//fmt.Printf("CompletionMode %d \n", pipeInfo.CurrentInstances)
	//fmt.Printf("CurrentInstances %d \n", pipeInfo.CurrentInstances)
	//fmt.Printf("MaximumInstances %d \n", pipeInfo.MaximumInstances)
	//fmt.Printf("InboundQuota %d \n", pipeInfo.InboundQuota)
	//fmt.Printf("ReadDataAvailable %d \n", pipeInfo.ReadDataAvailable)
	//fmt.Printf("OutboundQuota %d \n", pipeInfo.OutboundQuota)
	//fmt.Printf("WriteQuotaAvailable %d \n", pipeInfo.WriteQuotaAvailable)
	//fmt.Printf("PipeState %d \n", pipeInfo.PipeState)
	//fmt.Printf("PipeEnd %d \n", pipeInfo.PipeEnd)

	isexit <- true
}
