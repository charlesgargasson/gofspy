package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	FILE_NOTIFY_CHANGE_FILE_NAME   = 0x00000001
	FILE_NOTIFY_CHANGE_DIR_NAME    = 0x00000002
	FILE_NOTIFY_CHANGE_ATTRIBUTES  = 0x00000004
	FILE_NOTIFY_CHANGE_SIZE        = 0x00000008
	FILE_NOTIFY_CHANGE_LAST_WRITE  = 0x00000010
	FILE_NOTIFY_CHANGE_LAST_ACCESS = 0x00000020
	FILE_NOTIFY_CHANGE_CREATION    = 0x00000040
	FILE_NOTIFY_CHANGE_SECURITY    = 0x00000100
)

const (
	FILE_ACTION_ADDED            = 0x00000001
	FILE_ACTION_REMOVED          = 0x00000002
	FILE_ACTION_MODIFIED         = 0x00000003
	FILE_ACTION_RENAMED_OLD_NAME = 0x00000004
	FILE_ACTION_RENAMED_NEW_NAME = 0x00000005
	FILE_ACTION_STARTING_GOFSPY  = 0x10101010
)

func getActionType(action uint32, monitortype int) (string, bool) {
	switch action {
	case FILE_ACTION_ADDED:
		if monitortype == 1 || monitortype == 2 {
			return "🟢", false
		}
		return "🟢", true

	case FILE_ACTION_REMOVED:
		if monitortype == 1 || monitortype == 2 {
			return "❌", false
		}
		return "❌", false

	case FILE_ACTION_MODIFIED:
		if monitortype == 1 || monitortype == 2 {
			return "🟠", false
		}
		return "🟠", true

	case FILE_ACTION_RENAMED_OLD_NAME:
		return "🟣", false

	case FILE_ACTION_RENAMED_NEW_NAME:
		if monitortype == 1 || monitortype == 2 {
			return "🔵", false
		}
		return "🔵", true

	case FILE_ACTION_STARTING_GOFSPY:
		if monitortype == 1 {
			return "⚪", false
		}
		return "⚪", true

	default:
		return "?", false
	}
}

func handleFile(path string, action uint32, monitortype int, givenTime time.Time) {
	var owner string
	var displayAccess string
	var hijackable string
	actiontype, testAccess := getActionType(action, monitortype)
	emoji := "📁"
	if monitortype == 1 || monitortype == 2 {
		emoji = "💧"
	}

	if monitortype == 1 || monitortype == 2 {
		path = strings.Replace(path, `\\.\pipe\\`, `\\.\pipe\`, -1)
	}

	// Named pipes Hijack
	if hijack > 0 && (monitortype == 1 || monitortype == 2) && (action == FILE_ACTION_ADDED || action == FILE_ACTION_STARTING_GOFSPY) {
		// Check if Hijackable
		hijackHandle, err := createDuplexPipe(path)
		if err == nil {
			if hijack == 2 {
				windows.CloseHandle(hijackHandle)
				defer startServerHJ(path)
			} else {
				go windows.CloseHandle(hijackHandle)
			}
			hijackable = "🔥 "
		}
	}

	if !testAccess {
		fmt.Printf("%s %s %-2s %s %s%s\n", emoji, timeFormat(givenTime), displayAccess, actiontype, hijackable, path)
		return
	}

	// Named pipes
	if monitortype == 1 || monitortype == 2 {
		// Get access list and valid handle
		handle, _, _, _, controlAccess, displayAccess := bestFileHandle(path)
		defer func() {
			go windows.CloseHandle(handle)
		}()

		// Get owner
		if controlAccess {
			var wg sync.WaitGroup
			wg.Add(1)
			var owner string
			go getHandleOwner(handle, &owner, &wg)
			wg.Wait()
		}
		fmt.Printf("%s %s %-2s %s %s%s%s\n", emoji, timeFormat(givenTime), displayAccess, actiontype, hijackable, owner, path)
		return
	}

	// Start to search owner
	owner_ch := make(chan string)
	go getFileOwner(path, owner_ch)

	// Retrieve RW acess infos
	fileAttr, err := os.Stat(path)
	if err == nil {
		if !fileAttr.IsDir() {
			_, _, displayAccess = checkFileAccess(path)
		} else {
			displayAccess = "📁"
		}
		owner = <-owner_ch
		fmt.Printf("%s %s %s %s %s%s%s\n", emoji, timeFormat(givenTime), displayAccess, actiontype, hijackable, owner, path)
		return
	}

	owner = <-owner_ch
	fmt.Printf("%s %s %-2s %s %s%s%s\n", emoji, timeFormat(givenTime), displayAccess, actiontype, hijackable, owner, path)
}

func monitorpath(path string, monitortype int) {
	dirHandle, err := syscall.CreateFile(
		syscall.StringToUTF16Ptr(path),
		syscall.FILE_LIST_DIRECTORY,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)

	defer syscall.CloseHandle(dirHandle)

	if err != nil {
		fmt.Println("Error opening directory:", err)
		return
	}

	for {
		buffer := make([]byte, 4096)
		var bytesReturned uint32
		ret, _, _ := procReadDirectoryChangesW.Call(
			uintptr(dirHandle),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(len(buffer)),
			1, // Watch subtree
			FILE_NOTIFY_CHANGE_FILE_NAME|FILE_NOTIFY_CHANGE_DIR_NAME|
				FILE_NOTIFY_CHANGE_ATTRIBUTES|FILE_NOTIFY_CHANGE_SIZE|
				FILE_NOTIFY_CHANGE_LAST_WRITE|FILE_NOTIFY_CHANGE_CREATION,
			uintptr(unsafe.Pointer(&bytesReturned)),
			0,
			0,
		)
		if ret == 0 {
			fmt.Println("Failed to monitor directory")
			break
		}

		go func() {
			currentTime := time.Now()
			offset := 0
			for offset < int(bytesReturned) {
				record := (*syscall.FileNotifyInformation)(unsafe.Pointer(&buffer[offset]))
				action := record.Action
				fileName := utf16ToString(&record.FileName, record.FileNameLength)
				fullname := path + fileName
				go handleFile(fullname, action, monitortype, currentTime)

				if record.NextEntryOffset == 0 {
					break
				}
				offset += int(record.NextEntryOffset)
			}
		}()
	}
}

func monitornamedpipes(checkAccess bool, quitAfterList bool) {
	// Specify the folder path
	path := `\\.\pipe\`

	// Read the directory contents
	files, err := os.ReadDir(path)
	if err != nil {
		fmt.Printf("Error reading directory: %v\n", err)
		return
	}

	currentTime := time.Now()
	var action uint32
	action = FILE_ACTION_STARTING_GOFSPY
	monitortype := 1
	if checkAccess {
		monitortype = 2
	}

	var wg sync.WaitGroup

	// Print each named pipe
	for _, file := range files {
		fullname := path + file.Name()
		wg.Add(1)
		go func() {
			handleFile(fullname, action, monitortype, currentTime)
			defer wg.Done()
		}()
	}

	if !quitAfterList {
		go monitorpath(path, 1)
		select {}
	}

	wg.Wait()
}
