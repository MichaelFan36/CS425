package main

import (
	"bufio"
	"bytes"
	"container/list"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"sort"
	"strconv"
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

//transactionid map the whole transaction message
var transactionmap = make(map[string]string)

var localmempool *StringSet

// hash to find prevhash key:hash value:hash
var blockmap = make(map[string]string)

// hash to find if verified
var verifiedblockmap = make(map[string]bool)

var interrupted bool = false
var localtranlist = make([]string, 0)
var NodeInlock sync.Mutex
var NodeOutlock sync.Mutex
var MulticastListlock sync.Mutex
var localMempoollock sync.RWMutex
var loglock sync.Mutex
var Balancelock sync.Mutex
var Blockchain []Block
var tentativeBlock *Block

// Balance used to store balance   string(username)->balance
var Balance = make(map[string]int)

var node_name string
var client_port string
var err error
var localIp string
var message string

// node failure requester group: either inbound or outbound
var requester_group string

var MulticastListCond *sync.Cond
var NeedNewNode *sync.Cond
var VerifyMap = make(map[string]chan bool)

// Print Balance for checking consensus
func printBalance() {
	for !interrupted {
		fmt.Println("Balance info: ")
		for k, v := range Balance {
			fmt.Println("Username: ", k, "Balance: ", v)
		}
		duration := time.Duration(5) * time.Second
		time.Sleep(duration)
	}
}

/*
Nakamoto Consensus Infrastructure

1. mempool (string array sorted by time)
2. localmempool_cleaner

*/
type StringSet struct {
	mempool map[string]bool
}

func NewStringSet() *StringSet {
	NewStringSet := new(StringSet)
	NewStringSet.mempool = make(map[string]bool)
	return NewStringSet
}

type mempool []string

// return length
func (pool mempool) Len() int {
	return len(pool)
}

// swap i term and j term
func (pool mempool) Swap(i, j int) {
	pool[i], pool[j] = pool[j], pool[i]
}

// return true is i is greater than j
func (pool mempool) Less(i, j int) bool {
	T_i := strings.Split(pool[i], " ")[1]
	T_j := strings.Split(pool[j], " ")[1]
	T_i_num, _ := strconv.ParseFloat(T_i, 64)
	T_j_num, _ := strconv.ParseFloat(T_j, 64)
	return T_i_num < T_j_num
}

func (ss *StringSet) ChangetoArray() []string {
	// localMempoollock.RLock()
	// defer localMempoollock.RUnlock()
	array := make([]string, 0)
	for k := range ss.mempool {
		if len(array) < 2000 {
			array = append(array, k)
		} else {
			break
		}
	}
	return array
}

func (ss *StringSet) localmempool_cleaner() {
	// localMempoollock.Lock()
	for key, val := range ss.mempool {
		if val == true {
			delete(ss.mempool, key)
		}
	}
	// localMempoollock.Unlock()
}

/*
Part 2: Implementation of Nakamoto Consensus



*/
// declare struct block
type Block struct {
	height          int
	PreviousHash    string
	TransactionList []string
	Solution        string
	hash            string
	address         string
}

// block constructor
func newBlock(height int, PreviousHash string, TransactionList []string) *Block {
	block := new(Block)
	block.height = height
	block.PreviousHash = PreviousHash
	block.TransactionList = TransactionList
	block.Solution = ""
	block.hash = ""
	block.address = localIp + ":" + client_port
	return block
}

// use to get the puzzle of a block minusing the solution
func (block *Block) puzzle() string {
	var b bytes.Buffer
	e := gob.NewEncoder(&b)
	PreviousSolution := block.Solution
	block.Solution = ""
	if err := e.Encode(block); err != nil {
		panic(err)
	}
	h := sha256.New()
	h.Write(b.Bytes())
	block.Solution = PreviousSolution
	byteArray := h.Sum(nil)
	puzzle := hex.EncodeToString(byteArray)
	return puzzle
}

// use to get the puzzle of a block
func (block *Block) hashBlock() string {
	var b bytes.Buffer
	e := gob.NewEncoder(&b)
	if err := e.Encode(block); err != nil {
		panic(err)
	}
	h := sha256.New()
	h.Write(b.Bytes())
	byteArray := h.Sum(nil)
	puzzle := hex.EncodeToString(byteArray)
	return puzzle
}

// encode the block and send to conn
func SendBlock(conn net.Conn, block *Block) {
	encoder := gob.NewEncoder(conn)
	gossipMesg := "BLOCK" + "\n"
	fmt.Fprintf(conn, gossipMesg)
	encoder.Encode(*block)
}

// encode the block and send to conn
func SendUpdateBlock(conn net.Conn, block *Block) {
	encoder := gob.NewEncoder(conn)
	gossipMesg := "UPDATEBLOCK" + "\n"
	fmt.Fprintf(conn, gossipMesg)
	encoder.Encode(*block)
}

func SendMap(conn net.Conn, Balance map[string]int) {
	encoder := gob.NewEncoder(conn)
	gossipMesg := "Synchronized" + "\n"
	fmt.Fprintf(conn, gossipMesg)
	encoder.Encode(Balance)
}

// decode the block as received
func DecodeBlock(reader *bufio.Reader) *Block {
	decoder := gob.NewDecoder(reader)
	block := &Block{}
	err := decoder.Decode(block)
	if err != nil {
		fmt.Println("Decode Block Error:", err)
	}
	return block
}

func DecodeMap(reader *bufio.Reader) {
	decoder := gob.NewDecoder(reader)
	temp := make(map[string]int)
	err := decoder.Decode(temp)
	if err != nil {
		fmt.Println("Decode Map Error:", err)
	}
	Balance = temp
}

// verify the received block is correct
func VerifyBlock(block *Block) bool {
	Blockpuzzle := block.puzzle()
	Blocksolution := block.Solution
	fmt.Fprintf(introserver, "VERIFY"+" "+Blockpuzzle+" "+Blocksolution+"\n")
	channel := make(chan bool)
	VerifyMap[Blockpuzzle] = channel
	verified := <-channel
	if verified {
		if !verifiedblockmap[Blockpuzzle] {
			verified = false
		}
	}
	return verified
}

// func recursiveCheckAccount(currBlock *Block) bool {
// 	if currBlock.PreviousHash != "" {
// 		result := recursiveCheckAccount(currBlock.PreviousHash)
// 		if result == false {
// 			return false
// 		}
// 	}
// 	i := 0
// 	for i < len(currBlock.TransactionList) {
// 		if !updateAccount(currBlock.TransactionList[i]) {
// 			return false
// 		}
// 		i++
// 	}
// 	return true
// }

// func updateAccountFromChain(currBlock *Block) bool {
// 	account = make(map[string]int)

// 	return recursiveUpdateAccount(currBlock)
// }

// solve the current puzzle

func Len(transactionlist []string) int {
	return len(transactionlist)
}

func solvePuzzle() {
	localMempoollock.Lock()
	current_height := len(Blockchain)
	PreviousHash := ""
	if current_height == 0 {
		PreviousHash = ""
	} else {
		PreviousHash = Blockchain[len(Blockchain)-1].hashBlock()
	}
	localtranlist := localmempool.ChangetoArray()
	sort.Sort(mempool(localtranlist))
	tentativeBlock = newBlock(current_height+1, PreviousHash, localtranlist)
	fmt.Fprintf(introserver, "SOLVE"+" "+tentativeBlock.puzzle()+"\n")
	localMempoollock.Unlock()
}

// update block chain
func updateBlockChain(msgtype string, new_block *Block) {
	// either a locally generated block or a first block generated or no need for chain switching
	if msgtype == "local" ||
		new_block.height == 1 && tentativeBlock.height == 1 ||
		(tentativeBlock.height > 1 && new_block.height == tentativeBlock.height && tentativeBlock.PreviousHash == new_block.PreviousHash) {
		Blockchain = append(Blockchain, *new_block)
		for _, transaction_action := range new_block.TransactionList {
			Balancelock.Lock()
			updateBalance(transaction_action, Balance)
			Balancelock.Unlock()
			localMempoollock.Lock()
			localmempool.mempool[transaction_action] = true
			localmempool.localmempool_cleaner()
			localMempoollock.Unlock()

		}

	} else if msgtype == "unlocal" {
		for key, _ := range localmempool.mempool {
			localMempoollock.Lock()
			localmempool.mempool[key] = false
			localMempoollock.Unlock()
		}
		source_addr := new_block.address
		conn, err := net.Dial("tcp", source_addr)
		if err != nil {
			fmt.Println("Unable to dial for chain splitting\n")
		}
		// ROW BACK FALSE
		if new_block.height > tentativeBlock.height {
			for iter := tentativeBlock.height; iter < new_block.height; iter++ {
				//TODO updateBlock
				localMempoollock.Lock()
				s := strconv.Itoa(iter - 1)
				requested_block := requestBlock(conn, s)
				for _, transaction_action := range requested_block.TransactionList {
					localmempool.mempool[transaction_action] = true
				}
				localmempool.localmempool_cleaner()
				Blockchain = append(Blockchain, *requested_block)
				localMempoollock.Unlock()
			}
		}
		fmt.Fprintf(conn, "Synchronize"+"\n")
	}

	// if (local|| || next local generated block) {
	// 	node.append(new_block)
	// 	for transaction in new_block {
	// 		node.mempool.delete(transaction)
	// 	}
	// 	updateBalance(node, new_block)
	// } else {
	// 	// fmt.Println("Chain Switching\n")
	// 	requestmergeinfo(node, new_block) //getting new_block's mempool and balance to update node's
	// 	if(new_block.height > node.height +1 ){
	// 		verified
	// 		keep appending until reach the end of new_block.height
	// 	}
	// 	append (last one) new_block to node
	// 	update current height
	// 	for {
	// 		tranverse back from node to set blockchain linked list pre pointer to agree with blocchain array
	// 	}
	// }
	prevhash := new_block.hash
	current_height := new_block.height
	localtranlist := localmempool.ChangetoArray()
	sort.Sort(mempool(localtranlist))
	tentativeBlock = newBlock(current_height+1, prevhash, localtranlist)
	fmt.Fprintf(introserver, "SOLVE"+" "+tentativeBlock.puzzle()+"\n")
}

// TODO return the Block corrsponding to index i in Blockchain array of conn.RemoteAddr()
func requestBlock(conn net.Conn, i string) *Block {
	fmt.Fprintf(conn, "Request"+" "+i+"\n")
	reader := bufio.NewReader(conn)
	for !interrupted {
		netData, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("Failed to connect to request client")
			os.Exit(1)
		}
		if netData == "" || netData == "\n" {
			continue
		}
		if netData[len(netData)-1] == '\n' {
			netData = netData[0 : len(netData)-1]
		}
		sList := strings.Split(netData, " ")
		msgtype := sList[0]
		if msgtype == "UPDATEBLOCK" {
			block := DecodeBlock(reader)
			return block
		}
	}
	return nil
}

// update balance (only return true when there is a successful transaction being made)
func updateBalance(msg string, Balance map[string]int) bool {
	sList := strings.Split(msg, " ")
	Behavior := sList[0]
	fmt.Println(msg)
	if Behavior == "TRANSACTION" {

		source := sList[3]
		destination := sList[4]
		money := sList[5]
		money_txt := strings.Split(money, "\n")[0]
		CMoney, _ := strconv.ParseFloat(money_txt, 64)
		fmt.Println(source + "_" + destination + "_" + money_txt)
		fmt.Println(CMoney)
		if _, ok := Balance[source]; !ok {
			Balance[source] = 0
			Balance[destination] += int(CMoney)
			return true
		} else if Balance[source] >= int(CMoney) {
			Balance[source] -= int(CMoney)
			Balance[destination] += int(CMoney)
			return true
		}
	}
	fmt.Println("balance_after:")
	fmt.Println(Balance)
	return false
}

// how to handle block when recevied
func blockHandler() {

}

/*
Part 1: Transaction propagation
Message package:
PEERINTRODUCE
DESC
TRANSACTION

BLOCK

updating
updating_new
UPDATING
UPDATING_NEW
NEWNODE
NONE

Synchronized
Synchronize

*/
func push() {
	for !interrupted {
		MulticastListCond.L.Lock()
		for MulticastList.Len() == 0 {
			MulticastListCond.Wait()
		}
		msg := MulticastList.Front().Value.(string)
		MulticastList.Remove(MulticastList.Front())
		MulticastListCond.L.Unlock()

		fmt.Printf("enter push \n")
		// fmt.Printf("MutliscastList len: %d\n", MulticastList.Len())
		NodeOutlock.Lock()
		ele := connListOut.Front()
		fmt.Printf("outbound list size for %s: %d \n", node_name, connListOut.Len())
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
			//fmt.Println(node_name, "SENT DESC MESSAGE")
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
		//fmt.Println(netData)
		sList := strings.Split(netData, " ")
		msgtype := sList[0]
		//logging("C %d %s %s\n", len(netData)+1, getTimeString(), netData)
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

				Balancelock.Lock()
				updateBalance(netData, Balance)
				Balancelock.Unlock()

				MulticastListCond.L.Lock()
				MulticastList.PushBack(netData)
				MulticastListCond.L.Unlock()
				MulticastListCond.Signal()
				transactionmap[sList[2]] = netData
				multicastmap[sList[2]] = true
				//logging("T %s %s %s\n", node_name, getTimeString(), sList[1])
				localMempoollock.Lock()
				if _, ok := localmempool.mempool[netData]; !ok {
					localmempool.mempool[netData] = false
				}
				localMempoollock.Unlock()
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
		} else if msgtype == "Synchronize" {
			fmt.Println("Receive Synchronize MESSAGE ", netData)
			SendMap(conn, Balance)
		} else if msgtype == "Synchronized" {
			fmt.Println("Receive Synchronized MESSAGE ", netData)
			DecodeMap(reader)
		} else if msgtype == "Request" {
			index, _ := strconv.Atoi(sList[1])
			block := newBlock(1, "", make([]string, 0))
			block.height = Blockchain[index].height
			block.PreviousHash = Blockchain[index].PreviousHash
			block.TransactionList = Blockchain[index].TransactionList
			block.Solution = Blockchain[index].Solution
			block.hash = Blockchain[index].hash
			block.address = Blockchain[index].address
			SendUpdateBlock(conn, block)
		} else if msgtype == "BLOCK" {
			block := DecodeBlock(reader)
			updateBlockChain("unlocal", block)
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
	//("entered startconnection")
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
		//fmt.Println("CONNECT" + " " + node_name + " " + localIp + " " + client_port)
		fmt.Fprintf(introserver, "CONNECT"+" "+node_name+" "+localIp+" "+client_port+"\n")
	}
	// fmt.Fprintf(introserver, "SOLVE"+" "+""+"\n")
	// tentativeBlock = newBlock(1, "", make([]string, 0))
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
		fmt.Printf("Inbound list size for %s: %d \n", node_name, connListIn.Len())
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
	localmempool = NewStringSet()
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

	//print balance
	go printBalance()
	reader := bufio.NewReader(introserver)
	for !interrupted {
		//fmt.Println("entered for loop")
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
		fmt.Println(netData)
		sList := strings.Split(netData, " ")
		msgtype := sList[0]
		if msgtype == "INTRODUCE" {
			//fmt.Println("Receive INTRODUCE MESSAGE")
			newNodeIp := sList[2]
			go tryDial(newNodeIp, msgtype)
			// if connListOut.Len() == 1 {
			// 	go AcceptConnection(hostserver)
			// 	go push()
			// }
		} else if msgtype == "TRANSACTION" {
			if _, exist := multicastmap[sList[2]]; !exist {
				fmt.Println("Entered transaction")
				Balancelock.Lock()
				updateBalance(netData, Balance)
				Balancelock.Unlock()
				// multicast(netData)
				MulticastListCond.L.Lock()
				MulticastList.PushBack(netData)
				MulticastListCond.L.Unlock()
				MulticastListCond.Signal()
				transactionmap[sList[2]] = netData
				multicastmap[sList[2]] = true
				//logging("T %s %s %s\n", node_name, getTimeString(), sList[1])
				localMempoollock.Lock()
				if _, ok := localmempool.mempool[netData]; !ok {
					localmempool.mempool[netData] = false
				}
				PreviousHash := ""
				localtranlist := localmempool.ChangetoArray()
				sort.Sort(mempool(localtranlist))
				tentativeBlock = newBlock(1, PreviousHash, localtranlist)
				fmt.Fprintf(introserver, "SOLVE"+" "+tentativeBlock.puzzle()+"\n")
				fmt.Println("Sent SOLVE MESSAGE ", "SOLVE"+" "+tentativeBlock.puzzle())
				localMempoollock.Unlock()
			}
		} else if msgtype == "SOLVED" {
			fmt.Println("Receive SOLVED MESSAGE ", netData)
			tentativeBlock.Solution = sList[2]
			hash := tentativeBlock.hashBlock()
			tentativeBlock.hash = hash
			updateBlockChain("local", tentativeBlock)
		} else if msgtype == "VERIFY" {
			fmt.Println("Receive VERIFY MESSAGE ", netData)
			if sList[1] == "OK" {
				VerifyMap[sList[2]] <- true
			} else {
				VerifyMap[sList[2]] <- false
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
