package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/Microsoft/go-winio"
)

func readFromConn(conn net.Conn) (int, []byte, error) {
	var bufferSize int = 1024
	var data []byte
	for {
		buffer := make([]byte, bufferSize)
		n, err := conn.Read(buffer)
		if err != nil {
			return len(data), data, err
		}
		data = append(data, buffer[:n]...)
		if n < bufferSize {
			break
		}
	}

	return len(data), data, nil
}

func chatWithPipe(pipeName string) {
	conn, err := winio.DialPipe(pipeName, nil)
	if err != nil {
		fmt.Printf("💧 %s 🔴 Can't connect (%v)\n", timeFormat(time.Now()), err)
		return
	}
	defer conn.Close()

	fmt.Printf("💧 %s ⚪ Connected to %s\n", timeFormat(time.Now()), pipeName)

	var input []rune
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(conn)

	go func() {
		for {
			dataLen, data, err := readFromConn(conn)
			if err != nil {
				fmt.Printf("\n💧 %s 🔴 Can't read (%v)\n", timeFormat(time.Now()), err)
				return
			}

			if dataLen > 0 {
				// Print the data read from the named pipe
				fmt.Printf("\n💧 %s 🟢 received %d bytes : %q", timeFormat(time.Now()), dataLen, data)
				fmt.Printf("\n💧 >>")
			} else {
				time.Sleep(300 * time.Millisecond)
			}
		}
	}()

	for {
		fmt.Printf("\n💧 >> ")
		for {
			r, _, err := reader.ReadRune()
			if err != nil {
				fmt.Printf("\n💧 %s 🔴 Error reading keyboard input (%v) \n", timeFormat(time.Now()), err)
				return
			}
			if r == '\n' || r == '\r' { // Detect Enter key
				if r == '\r' {
					continue
				}
				if len(input) == 0 {
					fmt.Printf("💧 >> ")
					continue
				} else {
					break
				}
			} else {
				input = append(input, r) // Store the rune
			}
		}
		data := []byte(string(input))
		_, err := writer.Write(data) // _, err = conn.Write(data)
		if err != nil {
			fmt.Printf("💧 %s 🔴 Can't send data (%v) \n", timeFormat(time.Now()), err)
			return
		}
		writer.Flush()
		fmt.Printf("💧 %s 🟠 Sent %q", timeFormat(time.Now()), data)
		input = []rune{}
	}
}
