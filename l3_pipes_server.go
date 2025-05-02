package main

import (
	"context"
	"fmt"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func createDuplexPipe(pipeName string) (windows.Handle, error) {
	handle, err := windows.CreateNamedPipe(
		syscall.StringToUTF16Ptr(pipeName),
		windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_MESSAGE|windows.PIPE_READMODE_MESSAGE|windows.PIPE_WAIT,
		windows.PIPE_UNLIMITED_INSTANCES, //
		1024,                             // Output buffer size
		1024,                             // Input buffer size
		0,                                // Default timeout
		nil,                              // Security attributes
	)
	if err != nil {
		return handle, err
	}
	return handle, nil
}

func handleClientRead(handle windows.Handle, clientID int, ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Done()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return

		default:
			dataRead, err := readFromHandle(handle, false)
			if err != nil {
				fmt.Printf("💧 %s 🔴 [%03d] Can't read (%v) \n", timeFormat(time.Now()), clientID, err)
				return
			}

			dataLen := len(dataRead)
			if dataLen > 0 {
				fmt.Printf("💧 %s 🟢 [%03d] Received %d bytes: %q\n", timeFormat(time.Now()), clientID, len(dataRead), dataRead)
			} else {
				select {
				case <-ctx.Done():
					return
				case <-time.After(200 * time.Millisecond):
					//
				}
			}
		}
	}
}

func handleClientWrite(handle windows.Handle, clientID int, ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Done()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return

		default:
			dataWrite := fmt.Sprintf("Hello from pipe %d !\n", clientID)
			err := writeToHandle(handle, []byte(dataWrite), true)
			if err != nil {
				fmt.Printf("💧 %s 🔴 [%03d] Can't write (%v) \n", timeFormat(time.Now()), clientID, err)
				return
			}
			fmt.Printf("💧 %s 🟠 [%03d] Sent hello message \n", timeFormat(time.Now()), clientID)
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
				//
			}
		}
	}
}

func handleClient(handle windows.Handle, clientID int) {
	defer windows.CloseHandle(handle)
	fmt.Printf("💧 %s ⚪ [%03d] Connected client \n", timeFormat(time.Now()), clientID)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// wg.Add(2)
	wg.Add(1)

	// Start reader
	go handleClientRead(handle, clientID, ctx, cancel, &wg)

	// Start writer
	// go handleClientWrite(handle, clientID, ctx, cancel, &wg)

	wg.Wait()

	fmt.Printf("💧 %s ❌ [%03d] End client \n", timeFormat(time.Now()), clientID)
}

func startWorker(pipeName string, clientID *int) {
	for {
		var thisID int
		thisID = *clientID
		*clientID++
		handle, err := createDuplexPipe(pipeName)
		if err != nil {
			fmt.Printf("💧 %s 🔴 [%03d] Failed to start worker (%v)\n", timeFormat(time.Now()), thisID, err)
			windows.CloseHandle(handle)
			time.Sleep(1 * time.Second)
			continue
		}

		// fmt.Printf("💧 %s ⚪ [%03d] Started pipe \n", timeFormat(time.Now()), thisID)

		// Wait for a client to connect
		err = windows.ConnectNamedPipe(handle, nil)
		if err != nil {
			fmt.Printf("💧 %s 🔴 [%03d] Client failed to connect to pipe (%v)\n", timeFormat(time.Now()), thisID, err)
			windows.CloseHandle(handle)
		} else {
			go handleClient(handle, thisID)
		}
	}
}

func startServer(pipeName string, workers int) {
	fmt.Printf("💧 %s ⚪ Pipe server %s (%d workers) \n", timeFormat(time.Now()), pipeName, workers)
	clientID := 0
	for range workers {
		go startWorker(pipeName, &clientID)
		// time.Sleep(10 * time.Millisecond)
	}
	fmt.Printf("💧 %s ⚪ All workers are running \n", timeFormat(time.Now()))
	select {}
}
