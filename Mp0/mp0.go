package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var count = 0
var currtime = time.Now().Format("2006.01.02 15:04:05")
var path = currtime + "_logger_time.txt"
var nodemap = make(map[net.Conn]string)
var mtx sync.Mutex

func createFile() {
	fmt.Println("Got here")
	var _, err = os.Stat(path)
	if os.IsNotExist(err) {
		var file, err = os.Create(path)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer file.Close()
	}
}

func writeFile(text string) {
	var file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()
	_, err = file.WriteString(text)
	if err != nil {
		return
	}
	err = file.Sync()
	if err != nil {
		return
	}
}

func handleConnection(c net.Conn) {
	fmt.Print(".")
	reader := bufio.NewReader(c)
	for {
		netData, err := reader.ReadString('\n')
		if err != nil {
			time := fmt.Sprintf("%f", float64(time.Now().UnixNano())/float64(time.Second))
			netData = time + " - " + nodemap[c] + " " + "disconnected" + "\n"
			// mtx.Lock()
			// writeFile(netData)
			// mtx.Unlock()
			// fmt.Println(err)
			fmt.Print(netData)
			break
		}
		if _, ok := nodemap[c]; !ok {
			nodemap[c] = strings.Split(netData, " ")[2]
		}
		timestamps, err := strconv.ParseFloat(fmt.Sprintf("%f", float64(time.Now().UnixNano())/float64(time.Second)), 64)
		timestampc, err := strconv.ParseFloat(strings.Split(netData, " ")[0], 64)
		timedelay := fmt.Sprintf("%f", timestamps-timestampc) + "\n"
		bandwidth := fmt.Sprintf("%d", len(netData)) + "\n"
		// fmt.Println("Time Delay:", timestamps-timestampc)
		// fmt.Println("bandwidth is ", len(netData))
		fmt.Print(netData)
		mtx.Lock()
		writeFile(netData)
		writeFile(timedelay)
		writeFile(bandwidth)
		mtx.Unlock()
		counter := strconv.Itoa(count) + "\n"
		c.Write([]byte(string(counter)))
	}
	c.Close()
}

func main() {
	arguments := os.Args
	if len(arguments) == 1 {
		fmt.Println("Please provide a port number!")
		return
	}
	PORT := ":" + arguments[1]
	l, err := net.Listen("tcp4", PORT)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer l.Close()
	defer fmt.Println("Server Exited")
	createFile()
	for {
		c, err := l.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		go handleConnection(c)
		count++
	}
}
