package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	arguments := os.Args
	check := 0
	if len(arguments) == 1 {
		fmt.Println("Please provide host:port.")
		return
	}
	name := arguments[1]
	CONNECT := arguments[2]
	c, err := net.Dial("tcp", CONNECT)
	if err != nil {
		fmt.Println(err)
		return
	}

	for {
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		if check == 0 {
			fmt.Println("Got here client")
			sList := strings.Split(text, " ")
			timestamp := sList[0]
			data := sList[1]

			//newtext := timestamp + " - " + name + " " + "connected" + "\n" + timestamp + " - " + name + " " + data
			fmt.Fprintf(c, timestamp+" - "+name+" connected\n")

			// newtext2 := strings.Split(text, " ")[0] + " - " + name + " " + strings.Split(text, " ")[1]
			check++
			//fmt.Println(newtext)
			fmt.Fprintf(c, timestamp+" "+name+" "+data)
			// fmt.Fprintf(c, newtext2+"\n")
		} else {
			fmt.Println("Got in else")
			newtext := strings.Split(text, " ")[0] + " " + name + " " + strings.Split(text, " ")[1]
			fmt.Fprintf(c, newtext)
		}
	}
}
