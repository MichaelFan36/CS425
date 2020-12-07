package main

import (
	"bufio"
	"container/list"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const ARG_NUM_CLIENT int = 2

const AddrIntro string = "172.22.158.115:4990"
const AddrLogging string = "172.22.158.115:4991"

var large_num = 10000000000000
var maxIn = 127
var maxOut = 14
var conn net.Conn
var hostserver net.Conn
var introserver net.Conn
var logConn net.Conn
var connListIn, connListOut, MulticastList *list.List
var multicastmap = make(map[string]bool)
var transactionmap = make(map[string]string)

var interrupted bool = false

var NodeInlock sync.Mutex
var NodeOutlock sync.Mutex
var MulticastListlock sync.Mutex
var loglock sync.Mutex

var node_name string
var client_port string
var err error
var localIp string
var message string

// node failure requester group: either inbound or outbound
var requester_group string

var MulticastListCond *sync.Cond
var NeedNewNode *sync.Cond

func push() {
	for !interrupted {
		MulticastListCond.L.Lock()
		for MulticastList.Len() == 0 {
			MulticastListCond.Wait()
		}
		msg := MulticastList.Front().Value.(string)
		MulticastList.Remove(MulticastList.Front())
		MulticastListCond.L.Unlock()

		// fmt.Printf("enter push \n")
		// fmt.Printf("MutliscastList len: %d\n", MulticastList.Len())
		NodeOutlock.Lock()
		ele := connListOut.Front()
		// fmt.Printf("outbound list size for %s: %d \n", node_name, connListOut.Len())
		for ele != nil {
			next := ele.Next()
			_, e := fmt.Fprintf(ele.Value.(net.Conn), msg+"\n")
			fmt.Printf("multicast msg :%s \n", msg)
			if e != nil {
				connListOut.Remove(ele)
				NeedNewNode.Signal()
			}
			ele = next
		}
		NodeOutlock.Unlock()
	}
}

func pull() {
	for !interrupted {
		NeedNewNode.L.Lock()
		for connListOut.Len() == maxOut {
			NeedNewNode.Wait()
		}

		NodeOutlock.Lock()
		NodeInlock.Lock()
		NodeInLen := connListIn.Len()
		NodeOutLen := connListOut.Len()
		if NodeInLen != 0 && NodeOutLen != 0 {
			randNum := rand.Intn(large_num)
			if randNum%2 == 0 {
				requester_group = "inbound"
				randNum = rand.Intn(NodeInLen)
				ele := connListIn.Front()
				iter := 0
				for iter != randNum {
					ele = ele.Next()
					iter += 1
				}
				conn = ele.Value.(net.Conn)
			} else {
				requester_group = "outbound"
				randNum = rand.Intn(NodeOutLen)
				ele := connListOut.Front()
				iter := 0
				for iter != randNum {
					ele = ele.Next()
					iter += 1
				}
				conn = ele.Value.(net.Conn)
			}

			fmt.Fprintf(conn, "updating\n") //to do in handleconn
			NeedNewNode.L.Unlock()

			reader := bufio.NewReader(conn)
			message, err = reader.ReadString('\n')
			if err != nil {
				fmt.Printf("unable to update for %s", node_name)
			} else {
				sList := strings.Split(message, " ")
				msgtype := sList[0]
				dial_ip := sList[1]
				if requester_group == "inbound" {
					if msgtype == "NONE" {
						tryDial(dial_ip, "UPDATING")
					} else if msgtype == "NEWNODE" {
						tryDial(dial_ip, "UPDATING_NEW")
					}
				} else if requester_group == "outbound" {
					if msgtype == "NEWNODE" {
						tryDial(dial_ip, "UPDATING_NEW")
					}
				}
			}

		}
		NodeInlock.Unlock()
		NodeOutlock.Unlock()

	}
}

// fmt.Fprintf(conn, "NEWNODE"+" "+newNodeIp+"\n")
// 	fmt.Fprintf(conn, "NONE"+" "+localIp+"\n")

// func multicast(text string) {
// 	// MulticastListCond.L.Lock()
// 	MulticastListlock.Lock()
// 	MulticastList.PushBack(text)

// 	MulticastListlock.Unlock()
// 	// MulticastListCond.L.Unlock()
// 	// MulticastListCond.signal()
// }

func tryDial(client_addr string, msgtype string) {
	NodeOutlock.Lock()
	for node := connListOut.Front(); node != nil; node = node.Next() {
		if client_addr == node.Value.(net.Conn).RemoteAddr().String() {
			NodeOutlock.Unlock()
			return
		}
	}
	conn, err := net.Dial("tcp", client_addr+":"+client_port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s--- Cannot connect to %s\n", node_name, client_addr)
		NodeOutlock.Unlock()
	} else {
		if connListOut.Len() <= maxOut {
			node := connListOut.PushBack(conn)
			go handleConnection(conn, node)
			NodeOutlock.Unlock()
		} else {
			NodeOutlock.Unlock()
			conn.Close()
		}
		if msgtype == "INTRODUCE" {
			// fmt.Println(node_name, "SENT DESC MESSAGE")
			fmt.Fprintf(conn, "DESC"+" "+node_name+" "+localIp+"\n")
		}
		if msgtype == "PEERINTRODUCE" || msgtype == "DESC" {
			for _, v := range transactionmap {
				fmt.Fprintf(conn, v+"\n")
			}
		}
		if msgtype == "UPDATING" {
			for _, v := range transactionmap {
				fmt.Fprintf(conn, v+"\n")
			}
		}
		if msgtype == "UPDATING_NEW" {
			for _, v := range transactionmap {
				fmt.Fprintf(conn, v+"\n")
			}
			fmt.Fprintf(conn, "updating_new\n")
		}
	}

}

func logging(format string, a ...interface{}) {
	loglock.Lock()
	if logConn != nil {
		fmt.Fprintf(logConn, format, a...)
	}
	loglock.Unlock()
}

func handleConnection(conn net.Conn, node *list.Element) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for !interrupted {
		netData, err := reader.ReadString('\n')
		if err != nil {
			NodeInlock.Lock()
			connListIn.Remove(node)
			NodeInlock.Unlock()
			break
		}
		if netData == "" || netData == "\n" {
			continue
		}
		if netData[len(netData)-1] == '\n' {
			netData = netData[0 : len(netData)-1]
		}
		fmt.Println("Received from peers: " + netData)
		sList := strings.Split(netData, " ")
		msgtype := sList[0]
		logging("C %d %s %s\n", len(netData)+1, getTimeString(), netData)
		if msgtype == "PEERINTRODUCE" {
			//fmt.Println("Receive PEERINTRODUCE MESSAGE FROM", sList[2])
			newNodeIp := sList[2]
			go tryDial(newNodeIp, msgtype)
		} else if msgtype == "DESC" {
			//fmt.Println("Receive DESC MESSAGE FROM", sList[2])
			newNodeName := sList[1]
			newNodeIp := sList[2]
			if connListOut.Len() == 0 {
				go tryDial(sList[2], msgtype)

			} else {
				for node := connListOut.Front(); node != nil; node = node.Next() {
					fmt.Fprintf(node.Value.(net.Conn), "PEERINTRODUCE"+" "+newNodeName+" "+newNodeIp+"\n")
				}
			}
		} else if msgtype == "TRANSACTION" {
			if _, exist := multicastmap[sList[2]]; !exist {
				//multicast(netData)
				MulticastListCond.L.Lock()
				MulticastList.PushBack(netData)
				MulticastListCond.L.Unlock()
				MulticastListCond.Signal()
				transactionmap[sList[2]] = netData
				multicastmap[sList[2]] = true
				logging("T %s %s %s\n", node_name, getTimeString(), netData)
			}
		} else if msgtype == "updating" {
			if connListOut.Len() > 0 {
				randNum := rand.Intn(connListOut.Len())
				ele := connListOut.Front()
				iter := 0
				for iter != randNum {
					ele = ele.Next()
					iter += 1
				}
				newNodeIp := ele.Value.(net.Conn).RemoteAddr().String()
				fmt.Fprintf(conn, "NEWNODE"+" "+newNodeIp+"\n")
			} else {
				fmt.Fprintf(conn, "NONE"+" "+localIp+"\n")
				for _, v := range transactionmap {
					fmt.Fprintf(conn, v+"\n")
				}
			}
		} else if msgtype == "updating_new" {
			for _, v := range transactionmap {
				fmt.Fprintf(conn, v+"\n")
			}
		} else if msgtype == "QUIT" {
			interrupted = true
		} else if msgtype == "DIE" {
			os.Exit(2)
		} else {
			fmt.Println("message type not found.")
		}

	}
}

func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

func StartConnection(node_name string, client_port string) {
	// fmt.Println("entered startconnection")
	localIp = GetLocalIP()
	if localIp == "" {
		fmt.Println("Ip not found")
		return
	}

	logConn, err = net.Dial("tcp", AddrLogging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s--- Warning: cannot connect to log service\n", node_name)
	}

	introserver, err = net.Dial("tcp", AddrIntro)
	if err != nil {
		fmt.Println("Error, cannot dial server")
		os.Exit(1)
	} else {
		// fmt.Println("CONNECT" + " " + node_name + " " + localIp + " " + client_port)
		fmt.Fprintf(introserver, "CONNECT"+" "+node_name+" "+localIp+" "+client_port+"\n")
	}

}

func AcceptConnection(hostconn net.Listener) {
	for !interrupted {
		conn, err := hostconn.Accept()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Unable to accept clients: %s", node_name+"\n")
		}
		NodeInlock.Lock()
		if connListIn.Len() <= maxIn {
			node := connListIn.PushBack(conn)
			go handleConnection(conn, node)
		} else {
			conn.Close()
		}
		//fmt.Printf("Inbound list size for %s: %d \n", node_name, connListIn.Len())
		NodeInlock.Unlock()
	}
}

func getTimeString() string {
	return fmt.Sprintf("%f", float64(time.Now().UnixNano())/float64(time.Second))
}

func main() {
	argv := os.Args[1:]
	if len(argv) != ARG_NUM_CLIENT {
		fmt.Fprintf(os.Stderr, "usage: ./client <NODE_NAME> <SERV_PORT>\n")
		os.Exit(0)
	}

	node_name = argv[0]
	client_port = argv[1]

	// allowing keyboad interrupt
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	go func() {
		_ = <-sigs
		interrupted = true
	}()

	// data structure init
	MulticastListCond = sync.NewCond(&MulticastListlock)
	NeedNewNode = sync.NewCond(&NodeOutlock)
	connListIn = list.New()
	connListOut = list.New()
	MulticastList = list.New()

	// starting host server
	localIp = GetLocalIP()
	// hostserver, err := net.Listen("tcp", localIp+":"+client_port)
	hostserver, err := net.Listen("tcp", ":"+client_port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create host: %s\n", node_name)
	}
	StartConnection(node_name, client_port)

	defer hostserver.Close()
	go AcceptConnection(hostserver)
	go push()
	//go pull()
	reader := bufio.NewReader(introserver)
	for !interrupted {
		// fmt.Println("entered for loop")
		netData, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Failed to Connect to Intro Server")
			os.Exit(1)
		}
		if netData == "" || netData == "\n" {
			continue
		}
		if netData[len(netData)-1] == '\n' {
			netData = netData[0 : len(netData)-1]
		}
		fmt.Println("Received from introserver: " + netData)
		sList := strings.Split(netData, " ")
		msgtype := sList[0]
		if msgtype == "INTRODUCE" {
			// fmt.Println("Receive INTRODUCE MESSAGE")
			newNodeIp := sList[2]
			go tryDial(newNodeIp, msgtype)
			// if connListOut.Len() == 1 {
			// 	go AcceptConnection(hostserver)
			// 	go push()
			// }
		} else if msgtype == "TRANSACTION" {
			if _, exist := multicastmap[sList[2]]; !exist {
				// multicast(netData)
				MulticastListCond.L.Lock()
				MulticastList.PushBack(netData)
				MulticastListCond.L.Unlock()
				MulticastListCond.Signal()
				transactionmap[sList[2]] = netData
				multicastmap[sList[2]] = true
				logging("T %s %s %s\n", node_name, getTimeString(), netData)
			}
		} else if msgtype == "QUIT" {
			interrupted = true
		} else if msgtype == "DIE" {
			os.Exit(2)
		} else {
			fmt.Println("message type not found.")
		}

	}

}
