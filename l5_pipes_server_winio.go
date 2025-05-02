package main

import (
	"bufio"
	"context"
	"fmt"
	"sync"
	"time"

	"net"

	"github.com/Microsoft/go-winio"
)

func handleClientRead2(conn net.Conn, clientID int, ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Done()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return

		default:
			dataLen, data, err := readFromConn(conn)
			if err != nil {
				fmt.Printf("💧 %s 🔴 [%03d] Can't read (%v) \n", timeFormat(time.Now()), clientID, err)
				return
			}

			if dataLen > 0 {
				fmt.Printf("💧 %s 🟢 [%03d] Received %d bytes %q\n", timeFormat(time.Now()), clientID, dataLen, data)
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

func handleClientWrite2(conn net.Conn, clientID int, ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup) {
	defer wg.Done()
	defer cancel()
	writer := bufio.NewWriter(conn)
	time.Sleep(500 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			return

		default:
			data := fmt.Sprintf("Hello from pipe %d !\n", clientID)
			_, err := writer.Write([]byte(data)) //_, err := conn.Write([]byte(data))
			if err != nil {
				fmt.Printf("💧 %s 🔴 [%03d] Can't write (%v) \n", timeFormat(time.Now()), clientID, err)
				return
			}
			writer.Flush()
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

func handleClient2(conn net.Conn, clientID int) {
	fmt.Printf("💧 %s ⚪ [%03d] Connected client \n", timeFormat(time.Now()), clientID)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(2)

	// Start reader
	go handleClientRead2(conn, clientID, ctx, cancel, &wg)

	// Start writer
	go handleClientWrite2(conn, clientID, ctx, cancel, &wg)

	wg.Wait()

	conn.Close()
	fmt.Printf("💧 %s ❌ [%03d] Connection closed \n", timeFormat(time.Now()), clientID)
}

func startServer2(pipeName string) {
	listener, err := winio.ListenPipe(pipeName, nil)
	if err != nil {
		fmt.Printf("💧 %s 🔴 Failed to start server (%v)\n", timeFormat(time.Now()), err)
		return
	}
	fmt.Printf("💧 %s ⚪ Started server %s\n", timeFormat(time.Now()), pipeName)
	clientID := 0

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Printf("💧 %s 🔴 Failed to handle new client (%v)\n", timeFormat(time.Now()), err)
			continue
		}
		go handleClient2(conn, clientID) // Handle each client in a new goroutine
		clientID++
	}
}
