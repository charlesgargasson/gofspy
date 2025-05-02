package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

var debug bool
var helpmsg string = `
 Usage:

 📁 Monitoring (default)

    -files
        Files only 📁

    -pipes
        Named pipes only 💧
	
    -hijack int
        Try to start an instance for each pipe 💧
        1: Check only 
        2: Start MiTM

----------------------------------------------

 💧 Pipe Client

    -listpipes
        List pipes and quit

    -pipe string
        Pipe path

    -check
        Check access

    -read
        Stream

    -write string
        Write

    -writeread string
        Write data and stream responses

    -chat
        Start interactive chat

    -bytes
        Interpret escape sequences from input data

    -exhaust int
        🧪 exhaust pipe
        1: pool
        2: loop

----------------------------------------------

 💧 Pipe Server

    -server
        Start full duplex server using WinIO (1 worker)

    -nativeserver
        Start server using direct windows API calls (RO)

    -workers int
        Pool of workers for native server (default 4)

`

var version string = `
    ┏┓┏┓┏┓┏┓┏┓┓┏
    ┃┓┃┃┣ ┗┓┃┃┗┫
    ┗┛┗┛┻ ┗┛┣┛┗┛
            
         version 1.2.0

`

var hijack int

func main() {
	fmt.Printf("%s", version)
	defer fmt.Printf("\n")

	var usage string
	var files bool
	var pipes bool
	flag.BoolVar(&pipes, "pipes", false, usage)
	flag.BoolVar(&files, "files", false, usage)

	var listpipes bool
	flag.BoolVar(&listpipes, "listpipes", false, usage)

	var pipe string
	flag.StringVar(&pipe, "pipe", "", usage)

	var check bool
	flag.BoolVar(&check, "check", false, usage)

	// var debug bool
	flag.BoolVar(&debug, "debug", false, usage)

	var bytes bool
	flag.BoolVar(&bytes, "bytes", false, usage)

	var write string
	flag.StringVar(&write, "write", "", usage)

	var writeread string
	flag.StringVar(&writeread, "writeread", "", usage)

	var read bool
	flag.BoolVar(&read, "read", false, usage)

	var chat bool
	flag.BoolVar(&chat, "chat", false, usage)

	var nativeserver bool
	flag.BoolVar(&nativeserver, "nativeserver", false, usage)

	var server bool
	flag.BoolVar(&server, "server", false, usage)

	var workers int
	flag.IntVar(&workers, "workers", 4, usage)

	// var hijack int
	flag.IntVar(&hijack, "hijack", 0, usage)

	var exhaust int
	flag.IntVar(&exhaust, "exhaust", 0, usage)

	var help bool
	flag.BoolVar(&help, "help", false, usage)
	flag.BoolVar(&help, "h", false, usage)

	flag.Parse()

	if help {
		fmt.Printf(helpmsg)
		return
	}

	// exit channel for interactive modes
	isexit := make(chan bool)

	// SERVER MODE ///////////////////////

	if server {
		if pipe == "" {
			pipe = `\\.\pipe\testing`
		}
		startServer2(pipe)
		return
	}

	if nativeserver {
		if pipe == "" {
			pipe = `\\.\pipe\testing`
		}
		startServer(pipe, workers)
		return
	}

	// INTERACTIVE MODES ///////////////////////

	missingpipe := "[*] Missing pipe argument \n"

	if check && pipe != "" {
		go checkPipe(pipe, isexit)
		//go waitForExitInput(isexit)
		<-isexit
		return
	}

	if exhaust > 0 {
		if pipe == "" {
			fmt.Printf(missingpipe)
			return
		}
		go waitForExitInput(isexit)
		go exhaustPipe(pipe, exhaust)
		<-isexit
		return
	}

	if write != "" {
		if pipe == "" {
			fmt.Printf(missingpipe)
			return
		}
		if bytes {
			interpretedStr, err := strconv.Unquote(`"` + write + `"`)
			if err != nil {
				fmt.Println("Error interpreting escape sequences:", err)
				return
			}
			write = interpretedStr
		}
		go writeToPipe(pipe, []byte(write), isexit)
		//go waitForExitInput(isexit)
		<-isexit
		return
	}

	if writeread != "" {
		if pipe == "" {
			fmt.Printf(missingpipe)
			return
		}
		if bytes {
			interpretedStr, err := strconv.Unquote(`"` + writeread + `"`)
			if err != nil {
				fmt.Println("Error interpreting escape sequences:", err)
				return
			}
			writeread = interpretedStr
		}
		go writeReadToPipe(pipe, []byte(writeread), isexit)
		go waitForExitInput(isexit)
		<-isexit
		return
	}

	if read {
		if pipe == "" {
			fmt.Printf(missingpipe)
			return
		}
		go readFromPipe(pipe, isexit)
		go waitForExitInput(isexit)
		<-isexit
		return
	}

	if chat {
		if pipe == "" {
			fmt.Printf(missingpipe)
			return
		}
		chatWithPipe(pipe)
		return
	}

	if pipe != "" {
		fmt.Printf("[*] Missing action for pipe \n")
		return
	}

	// NORMAL MODES ////////////////////////////

	if !pipes && !files {
		pipes = true
		files = true
	}

	if listpipes {
		files = false
	}

	if pipes {
		go func() {
			monitornamedpipes(check, listpipes)
			isexit <- true
		}()
	}

	if files {
		for driveLetter := 'C'; driveLetter <= 'Z'; driveLetter++ {
			go func() {
				drivePath := fmt.Sprintf("%c:\\", driveLetter)
				if _, err := os.Stat(drivePath); err == nil {
					go monitorpath(drivePath, 0)
				}
			}()
		}
	}

	<-isexit
}
