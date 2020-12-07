package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

/*
    B timestamp length
	T nodename timestamp TRANSACTION createtime
*/

var path = "log.txt"
var RWLOCK sync.Mutex
var bandwidthMtx sync.Mutex

var totalConnect int32
var totalDisconnect int32

var BandWidth map[int64]int64

var file *(os.File)

var exitCond *sync.Cond
var exitMtx sync.Mutex

func handleConnection(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		netdata, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		dat := strings.Split(netdata[:len(netdata)-1], " ")
		if dat[0] == "B" {
			time, TimeError := strconv.ParseFloat(dat[1], 64)
			bandwidth, BandwidthError := strconv.ParseInt(dat[2], 10, 64)
			if TimeError != nil {
				fmt.Fprintf(os.Stderr, "Logger: Timestamp failed\n")
				continue
			}
			if BandwidthError != nil {
				fmt.Fprintf(os.Stderr, "Logger: Bandwidth failed\n")
				continue
			}
			bandwidthMtx.Lock()
			id := int64(time)
			totalBandwidth, exist := BandWidth[id]
			if !exist {
				totalBandwidth = 0
			}
			BandWidth[id] = totalBandwidth + bandwidth
			bandwidthMtx.Unlock()
		} else {
			RWLOCK.Lock()
			file.WriteString(netdata)
			RWLOCK.Unlock()
		}
	}
	atomic.AddInt32(&totalDisconnect, 1)
	exitCond.Signal()
}

func listen(listener net.Listener) {
	for {
		conn, _ := listener.Accept()
		atomic.AddInt32(&totalConnect, 1)
		go handleConnection(conn)
	}
}

func main() {
	exitCond = sync.NewCond(&exitMtx)
	BandWidth = make(map[int64]int64)

	listener, err := net.Listen("tcp", ":4991")
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger failed to listen\n")
		os.Exit(1)
	}

	var FileError error
	file, FileError = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if FileError != nil {
		fmt.Fprintf(os.Stderr, "logger failed to create file\n")
		os.Exit(1)
	}

	defer file.Close()
	go listen(listener)
	exitCond.L.Lock()
	for totalConnect == 0 || totalConnect != totalDisconnect {
		exitCond.Wait()
	}
	exitCond.L.Unlock()
	RWLOCK.Lock()
	for timeInt := range BandWidth {
		file.WriteString(fmt.Sprintf("B %d %d\n", timeInt, BandWidth[timeInt]))
	}
	RWLOCK.Unlock()
}
