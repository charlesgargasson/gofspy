package main

import (
	"fmt"
	"syscall"
	"time"
	"sync"

	"golang.org/x/sys/windows"
)

func exhaustPipe(pipeName string, exhaustType int) {
	// 1: exhaust pool, keep handles open
	// 2: speed exhaust, keep it stuck with many requests
	exhaustLimit := 4097
	exhaustcpt := 0
	failed := 0
	stuck := false
	if exhaustType == 1 {
		for exhaustcpt < exhaustLimit {
			// Open the named pipe with READ_CONTROL
			handle, err := windows.CreateFile(
				syscall.StringToUTF16Ptr(pipeName),
				windows.READ_CONTROL,
				0,
				nil,
				windows.OPEN_EXISTING,
				0,
				0,
			)
			defer windows.CloseHandle(handle)
			if err != nil {
				failed++
				if failed > 10 {
					fmt.Printf("💧 %s 🟢 Probably exhausted, with %d active handles (%v)\n", timeFormat(time.Now()), exhaustcpt, err)
					fmt.Println("Error message:", err)
					select {}
				}
			} else {
				exhaustcpt++
			}
			time.Sleep(10 * time.Millisecond)
		}
		fmt.Printf("💧 %s 🔴 Reached limit \n", timeFormat(time.Now()))

	} else if exhaustType == 2 {
		for {
			// Open the named pipe with READ_CONTROL
			handle, err := windows.CreateFile(
				syscall.StringToUTF16Ptr(pipeName),
				windows.READ_CONTROL,
				0,
				nil,
				windows.OPEN_EXISTING,
				0,
				0,
			)
			defer windows.CloseHandle(handle)
			if err != nil {
				failed++
				if !stuck && failed > 5 {
					fmt.Printf("💧 %s 🟢 Probably exhausted, %d active handles (%v)\n", timeFormat(time.Now()), exhaustcpt, err)
					stuck = true
				}
			} else {
				failed = 0
				exhaustcpt++
				if stuck {
					fmt.Printf("💧 %s 🔴 Server is alive again (%d active handles) \n", timeFormat(time.Now()), exhaustcpt)
					stuck = false
				}
			}
		}
	}
}

func checkPipe(pipeName string, isexit chan bool) {
	handle, readAccess, writeAccess, _, controlAccess, _ := bestFileHandle(pipeName)

	if hijack > 0 {
		hijackHandle, err := createDuplexPipe(pipeName)
		windows.CloseHandle(hijackHandle)
		if err != nil {
			fmt.Printf("💧 %s 🔴 Can't Hijack (%v)\n", timeFormat(time.Now()), err)
		} else {
			fmt.Printf("💧 %s 🟢 Hijackable \n", timeFormat(time.Now()))
		}
	}

	// Get informations of server pipe handle
	var wg sync.WaitGroup
	var pid uint32
	var owner string
	var namedPipeHandleState namedPipeHandleStateStruct

	if controlAccess {

		wg.Add(3)
		go GetNamedPipeServerPID(handle, &pid, &wg)
		go getHandleOwner(handle, &owner, &wg)
		go GetNamedPipeHandleState(handle, &namedPipeHandleState, &wg)

	}

	wg.Wait()
	
	windows.CloseHandle(handle)

	// print infos

	if controlAccess {
		if pid > uint32(0) {
			fmt.Printf("💧 %s ⚪ Pid: %d\n", timeFormat(time.Now()), pid)
		}
		if owner != "" {
			fmt.Printf("💧 %s ⚪ Owner: %s\n", timeFormat(time.Now()), owner)
		}
		if namedPipeHandleState.success {
			switch namedPipeHandleState.state {
			case uint32(0):
				fmt.Printf("💧 %s ⚪ State: WAIT\n", timeFormat(time.Now()))
			case windows.PIPE_NOWAIT:
				fmt.Printf("💧 %s ⚪ State: NOWAIT\n", timeFormat(time.Now()))
			case windows.PIPE_READMODE_MESSAGE:
				fmt.Printf("💧 %s ⚪ State: MESSAGE\n", timeFormat(time.Now()))
			}
			
			fmt.Printf("💧 %s ⚪ Pipes: %d\n", timeFormat(time.Now()), namedPipeHandleState.curInstances)
			//fmt.Printf("💧 %s ⚪ Max data: %d bytes\n", timeFormat(time.Now()), namedPipeHandleState.maxCollectionCount)
			//fmt.Printf("💧 %s ⚪ Timeout: %d\n", timeFormat(time.Now()), namedPipeHandleState.collectDataTimeout)
			//fmt.Printf("💧 %s ⚪ User: %s\n", timeFormat(time.Now()), namedPipeHandleState.userName)
		}
	}

	if readAccess {
		fmt.Printf("💧 %s 🟢 Readable \n", timeFormat(time.Now()))
	} else {
		fmt.Printf("💧 %s 🔴 Can't read \n", timeFormat(time.Now()))
	}

	if writeAccess {
		fmt.Printf("💧 %s 🟢 Writable \n", timeFormat(time.Now()))
	} else {
		fmt.Printf("💧 %s 🔴 Can't write \n", timeFormat(time.Now()))
	}

	// MiTM Server

	if hijack == 2 {
		fmt.Printf("\n")
		startServerHJ(pipeName)
	}

	isexit <- true
}

func readFromPipe(pipeName string, isexit chan bool) {
	// Open the named pipe
	handle, err := windows.CreateFile(
		syscall.StringToUTF16Ptr(pipeName),
		windows.GENERIC_READ,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)

	defer windows.CloseHandle(handle)

	if err != nil {
		fmt.Printf("💧 %s 🔴 Can't retrieve Read handle (%v)\n", timeFormat(time.Now()), err)
		isexit <- true
		return
	}

	fmt.Printf("💧 %s 🟢 Read handle \n", timeFormat(time.Now()))

	for {
		data, err := readFromHandle(handle, false)
		if err != nil {
			fmt.Printf("\n💧 %s 🔴 Can't read (%v)\n", timeFormat(time.Now()), err)
			isexit <- true
			return
		}

		// Print the data read from the named pipe
		fmt.Printf("💧 %s 🟠 received %d bytes %q\n", timeFormat(time.Now()), len(data), data)
	}

	isexit <- true
}

func writeToPipe(pipeName string, data []byte, isexit chan bool) {
	// Open the named pipe
	handle, err := windows.CreateFile(
		syscall.StringToUTF16Ptr(pipeName),
		windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)

	defer windows.CloseHandle(handle)

	if err != nil {
		fmt.Printf("💧 %s 🔴 Can't retrieve Write handle (%v)\n", timeFormat(time.Now()), err)
		isexit <- true
		return
	}

	fmt.Printf("💧 %s 🟢 Write handle on %s\n", timeFormat(time.Now()), pipeName)

	err = writeToHandle(handle, data, false)
	if err != nil {
		fmt.Printf("💧 %s 🔴 Can't send data (%v) \n", timeFormat(time.Now()), err)
		isexit <- true
		return
	}
	fmt.Printf("💧 %s 🟠 Sent %q\n", timeFormat(time.Now()), data)
	isexit <- true
}

func writeReadToPipe(pipeName string, data []byte, isexit chan bool) {
	handle, err := windows.CreateFile(
		syscall.StringToUTF16Ptr(pipeName),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)

	defer windows.CloseHandle(handle)

	if err != nil {
		fmt.Printf("💧 %s 🔴 Can't retrieve Read/Write handle (%v)\n", timeFormat(time.Now()), err)
		isexit <- true
		return
	}

	fmt.Printf("💧 %s 🟢 Read/Write handle on %s\n", timeFormat(time.Now()), pipeName)

	err = writeToHandle(handle, data, false)
	if err != nil {
		fmt.Printf("💧 %s 🔴 Can't send data (%v) \n", timeFormat(time.Now()), err)
		isexit <- true
		return
	}
	fmt.Printf("💧 %s 🟠 Sent: %q\n", timeFormat(time.Now()), data)

	for {
		data, err := readFromHandle(handle, false)
		if err != nil {
			fmt.Printf("\n💧 %s 🔴 Can't read (%v)\n", timeFormat(time.Now()), err)
			isexit <- true
			return
		}

		// Print the data read from the named pipe
		fmt.Printf("💧 %s 🟠 received %d bytes %q\n", timeFormat(time.Now()), len(data), data)
	}

	isexit <- true
}
