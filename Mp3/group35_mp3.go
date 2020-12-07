package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/twmb/algoimpl/go/graph"
)

// Constant
const CoordinatorAddr = "sp20-cs425-g35-02.cs.illinois.edu:8000"

// Count Down timer
const timeout = 1 * time.Minute

var start = time.Now() // starting time of the timer
var BEGIN_bool = false

var totaltransactions int

var clientlock sync.RWMutex

var clienttempstore map[string]map[string]string
var clientid string
var clientworking bool
var clientIsAborted bool
var clientCommandsqueue *CommandQueue
var clientReadLockSet *StringMap

/// UNCHANGED
// helper functions

// Credit: https://blog.golang.org/maps
// StringMap : key: transactionID  value: exist/no
type StringMap struct {
	set    map[string]bool
	RWlock sync.RWMutex
}

// Construntor for StringMap
func NewSet() *StringMap {
	s := new(StringMap)
	s.set = make(map[string]bool)
	return s
}

// Add
func (set *StringMap) SetAdd(s string) bool {
	set.RWlock.Lock()
	_, found := set.set[s]
	set.set[s] = true
	set.RWlock.Unlock()
	return !found
}

// Delete
func (set *StringMap) SetDelete(s string) bool {
	set.RWlock.Lock()
	defer set.RWlock.Unlock()
	_, found := set.set[s]
	if !found {
		return false
	}
	delete(set.set, s)
	return true
}

// Contain
func (set *StringMap) SetHas(s string) bool {
	set.RWlock.RLock()
	_, found := set.set[s]
	set.RWlock.RUnlock()
	return found
}

// Dump Set to an array
func (set *StringMap) SetToArray() []string {
	set.RWlock.RLock()
	defer set.RWlock.RUnlock()
	keys := make([]string, 0)
	for k := range set.set {
		keys = append(keys, k)
	}
	return keys
}

// Size
func (set *StringMap) Size() (size int) {
	set.RWlock.RLock()
	defer set.RWlock.RUnlock()
	size = len(set.set)
	return
}

// // GetRandom : Get a random element ORIGIN set
// func (set *StringMap) GetRandom() string {
// 	set.RWlock.RLock()
// 	defer set.RWlock.RUnlock()
// 	if len(set.set) == 0 {
// 		return ""
// 	}
// 	i := rand.Intn(len(set.set))
// 	for k := range set.set {
// 		if i == 0 {
// 			return k
// 		}
// 		i--
// 	}
// 	return ""
// }

// return the only element in the map
func (set *StringMap) Only() string {
	for k := range set.set {
		return k
	}
	return ""
}

// CommandQueue string slice
// Credit: https://blog.golang.org/slices-intro
// Client command Queue
type CommandQueue struct {
	Commands    []string
	NotifyChann chan bool
	lock        sync.RWMutex
}

// Constructor
func NewQueue() *CommandQueue {
	q := new(CommandQueue)
	q.Commands = make([]string, 0)
	q.NotifyChann = make(chan bool, 1)
	return q
}

// Push (to the end of the queue)
func (q *CommandQueue) Push(c string) {
	q.lock.Lock()
	if len(q.Commands) == 0 {
		q.NotifyChann <- true
	}
	q.Commands = append(q.Commands, c)
	q.lock.Unlock()
}

// Pop (ORIGIN the top of the queue)
func (q *CommandQueue) Pop() string {
	q.lock.Lock()
	res := q.Commands[0]
	q.Commands = q.Commands[1:len(q.Commands)]
	q.lock.Unlock()
	return res
}

// Check the queue is empty
func (q *CommandQueue) IsEmpty() bool {
	q.lock.RLock()
	defer q.lock.RUnlock()
	return len(q.Commands) == 0
}

// Clear the Queue
func (q *CommandQueue) Clear() {
	q.lock.Lock()
	q.Commands = make([]string, 0)
	q.lock.Unlock()
}

type Args struct {
	AccountName   string
	Value         string
	TransactionID string
}

type CoordinatorArgs struct {
	ORIGIN      string
	DESTINATION []string
}

//Credit: https://stackoverflow.com/questions/23558425/how-do-i-get-the-local-ip-address-in-go
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

// RPCToCoor  ... COMMENT (RPCTOCOORDINATOR)
func RPCToCoor(action string, transactionID string, destinationIDs []string) string {
	rpcClient, err := rpc.Dial("tcp", CoordinatorAddr)
	if err != nil {
		fmt.Println("dial error")
		fmt.Println(err)
		os.Exit(1)
	}
	args := &CoordinatorArgs{ORIGIN: transactionID, DESTINATION: destinationIDs}
	var response string
	if action == "BEGIN" {
		err = rpcClient.Call("Coordinator.NewTransaction", args, &response)
	} else if action == "REMOVE" {
		err = rpcClient.Call("Coordinator.RemoveTransaction", args, &response)
	} else {
		fmt.Println("Unknown action is used " + action)
	}
	if err != nil {
		fmt.Println("Coordinator error:")
		fmt.Println(err)
		os.Exit(1)
	}
	rpcClient.Close()
	return response
}

/// UNCHANGED

// server

type Server struct {
	Name     string
	Accounts map[string]*Account
}

// Server Constructor
func NewServer(name string) *Server {
	server := new(Server)
	server.Name = name
	server.Accounts = make(map[string]*Account)
	return server
}

//Account object
type Account struct {
	AccountBalance string
	Readers        *StringMap
	Writer         string
	RequestQueue   []*LockStat
	AccountLock    sync.Mutex
}

// Server Account constructor
func NewAccount(balance string) *Account {
	Account := new(Account)
	Account.AccountBalance = balance
	Account.Readers = NewSet()
	Account.Writer = ""
	Account.RequestQueue = make([]*LockStat, 0)
	return Account
}

// LockStat definition
type LockStat struct {
	Purpose       string // read, write, promote
	TransactionID string
	Status        chan bool // 1 for granting access 0 for denying access
}

// LockStat constructor
func NewLockStat(purpose string, tid string) *LockStat {
	lock := new(LockStat)
	lock.Purpose = purpose
	lock.TransactionID = tid
	lock.Status = make(chan bool, 1)
	return lock
}

// Start a new server with given name
func InitializeServer(Name string, port string) {
	server := NewServer(Name)
	rpc.Register(server)
	tcpAddress, err := net.ResolveTCPAddr("tcp", ":"+port)
	if err != nil {
		fmt.Println("Error:")
		fmt.Println(err)
		os.Exit(1)
	}
	listener, err := net.ListenTCP("tcp", tcpAddress)
	if err != nil {
		fmt.Println("Listener Error:\n")
		fmt.Println(err)
		os.Exit(1)
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Accept Error")
			continue
		}
		go rpc.ServeConn(conn)
	}
}

// Change account value on server based on Args (commit phase)
func (server *Server) Change(args *Args, response *string) error {
	obj, found := server.Accounts[args.AccountName]
	*response = "SUCCESS"
	fmt.Println("Write: TransactionID is: ", args.TransactionID)
	if !found {
		newObj := NewAccount(args.Value)
		server.Accounts[args.AccountName] = newObj
		newObj.Writer = args.TransactionID
		return nil
	}

	obj.AccountLock.Lock()
	if obj.Writer != args.TransactionID {
		//TODO: DEADLOCK HANDLING
		fmt.Println("No Write Lock for Committing")
	}
	obj.AccountBalance = args.Value
	obj.AccountLock.Unlock()

	return nil
}

// UpdateAccount ... Update different transaction IDs' permissions based on RequestQueue ; remove transaction ID from the argument in the account
// UpdataAccount: For cleaning up: delete current transaction ID (contained in args) from RequestQueue, Handling next request from requestQueue
func (server *Server) UpdateAccount(args *Args, response *string) error {
	obj, found := server.Accounts[args.AccountName]
	if !found {
		fmt.Println("UpdateAccount: Account accountName=" + args.AccountName + " not found")
		return nil
	}

	obj.AccountLock.Lock()
	obj.Readers.SetDelete(args.TransactionID)

	if obj.Writer == args.TransactionID {
		obj.Writer = ""
	}

	// fmt.Println("Writer is: ", obj.Writer)
	// fmt.Println("TransactionID is: ", args.TransactionID)
	// delete current transaction ID (contained in args) from RequestQueue
	for index, queuedReq := range obj.RequestQueue {
		if queuedReq.TransactionID == args.TransactionID {
			queuedReq.Status <- false
			obj.RequestQueue = append(obj.RequestQueue[:index], obj.RequestQueue[index+1:]...)
			break
		}
	}
	*response = "SUCCESS"

	// read & write handling
	if obj.Readers.Size() == 0 && obj.Writer == "" && len(obj.RequestQueue) > 0 {
		if obj.RequestQueue[0].Purpose == "write" {
			///Granting write access for only one request (transaction ID) a time
			req := obj.RequestQueue[0]
			obj.RequestQueue = obj.RequestQueue[1:]
			obj.Writer = req.TransactionID
			req.Status <- true
		} else if obj.RequestQueue[0].Purpose == "read" { //keep granting access to read request on the queue
			for len(obj.RequestQueue) > 0 && obj.RequestQueue[0].Purpose == "read" {
				req := obj.RequestQueue[0]
				obj.RequestQueue = obj.RequestQueue[1:]
				obj.Readers.SetAdd(req.TransactionID)
				req.Status <- true
			}
		}
	}

	// Promote
	if obj.Readers.Size() == 1 && obj.Writer == "" && len(obj.RequestQueue) > 0 {
		req := obj.RequestQueue[0]
		if req.Purpose == "promote" {
			// Promote from same transaction ID
			if req.TransactionID == obj.Readers.Only() {
				// fmt.Println("got here promotion")
				obj.RequestQueue = obj.RequestQueue[1:]
				obj.Readers.SetDelete(req.TransactionID)
				obj.Writer = req.TransactionID
				req.Status <- true //granting access
			} else {
				// TODO: Deadlock hanlding
				obj.AccountLock.Unlock()
				return errors.New("Unmatched promotion transaction ID ")
			}
		}
	}

	obj.AccountLock.Unlock()

	return nil
}

// WriteLockRequest .... Based on AccountName provided in argument, return
//   SUCCESS: write permission is granted (with server name, current accountname, and accountbalance) [current account balance delivery granted]
//   ABORT: Server is unable to grant permission
//   NOT FOUND: no existing account
//   errormsg
//   Grant only permission only to temp container (for balance updating) before (commit phase)
func (server *Server) WriteLockRequest(args *Args, response *string) error {
	obj, found := server.Accounts[args.AccountName]

	// Granting access to newly created account; return success
	if !found {
		*response = "NOT FOUND"
		return nil
	}

	obj.AccountLock.Lock()
	//There is no writer for this account
	if obj.Writer == "" {
		// Grant access when there is no reader and writer
		if obj.Readers.Size() == 0 {
			obj.Writer = args.TransactionID
			*response = "SUCCESS " + server.Name + "." + args.AccountName + " " + obj.AccountBalance
			obj.AccountLock.Unlock()
		} else {
			// Requester is the only reader
			if obj.Readers.SetHas(args.TransactionID) {
				if obj.Readers.Size() == 1 {
					// Same Transaction ID promotion (for only one reader no writer)
					obj.Writer = args.TransactionID
					obj.Readers.SetDelete(args.TransactionID)
					*response = "SUCCESS " + server.Name + "." + args.AccountName + " " + obj.AccountBalance
					obj.AccountLock.Unlock()
				} else {
					// More than one reader for the account
					// Wait until transaction is the only reader, then promote the reader to writer
					req := NewLockStat("promote", args.TransactionID)
					obj.RequestQueue = append([]*LockStat{req}, obj.RequestQueue...) // Put the request to the beginning of the queue
					obj.AccountLock.Unlock()

					// Waiting for access
					ok := <-req.Status
					if ok {
						*response = "SUCCESS " + server.Name + "." + args.AccountName + " " + obj.AccountBalance
					} else {
						*response = "ABORT"
					}
				}
			} else {
				// Requester is not the reader in the queue
				req := NewLockStat("write", args.TransactionID)

				// Append the request at the end of the queue
				obj.RequestQueue = append(obj.RequestQueue, req)

				obj.AccountLock.Unlock()

				// Wait for access
				ok := <-req.Status
				if ok {
					*response = "SUCCESS " + server.Name + "." + args.AccountName + " " + obj.AccountBalance
				} else {
					*response = "ABORT"
				}
			}
		}
	} else {
		// Some other client is holding the write lock
		// return reader-writer conflict when there are still readers (Technically this should never happen)
		if obj.Readers.Size() != 0 {
			fmt.Println("Reader-Writer Conflict!")
			obj.AccountLock.Unlock()
			return errors.New("Write: Account accountName=" + args.AccountName + ", Transaction=" + args.TransactionID + ". Reader-writer conflict.")
		}
		req := NewLockStat("write", args.TransactionID)

		//Append the request at the end of RequestQueue
		obj.RequestQueue = append(obj.RequestQueue, req)
		obj.AccountLock.Unlock()

		// Wait for access
		ok := <-req.Status
		if ok {
			*response = "SUCCESS " + server.Name + "." + args.AccountName + " " + obj.AccountBalance
		} else {
			*response = "ABORT"
		}
	}
	return nil
}

// ReadLockRequest : Handle Read Lock permissionn and read msg delivery for an account
//          SUCCESS + servername.accountname + " " + accountbalance
//          NOT FOUND : NO account exists
//          ABORT : Server is unable to grant permisiion
func (server *Server) ReadLockRequest(args *Args, response *string) error {
	obj, found := server.Accounts[args.AccountName]

	if !found {
		*response = "NOT FOUND"
		return nil
	}

	obj.AccountLock.Lock()
	// fmt.Println(obj.Readers.SetToArray())

	// No Readers and Writers
	if obj.Writer == "" && obj.Readers.Size() == 0 {
		obj.Readers.SetAdd(args.TransactionID)
		*response = "SUCCESS " + server.Name + "." + args.AccountName + " " + obj.AccountBalance
		obj.AccountLock.Unlock()

	} else if obj.Readers.Size() == 0 && obj.Writer != "" {
		//Someone is holding the write lock
		req := NewLockStat("read", args.TransactionID)
		obj.RequestQueue = append(obj.RequestQueue, req)
		obj.AccountLock.Unlock()
		// Wait for access
		ok := <-req.Status
		fmt.Println("No readers, has writer: Unblocked")
		if ok {
			*response = "SUCCESS " + server.Name + "." + args.AccountName + " " + obj.AccountBalance
		} else {
			*response = "ABORT"
		}
	} else if obj.Readers.Size() > 0 && obj.Writer == "" {
		// There are readers and no writer
		// (writer-priority RW lock): only grant more read if there no write request pending
		if len(obj.RequestQueue) == 0 {
			obj.Readers.SetAdd(args.TransactionID)
			*response = "SUCCESS " + server.Name + "." + args.AccountName + " " + obj.AccountBalance
			obj.AccountLock.Unlock()
		} else {
			req := NewLockStat("read", args.TransactionID)
			obj.RequestQueue = append(obj.RequestQueue, req)
			obj.AccountLock.Unlock()
			// Wait for access
			ok := <-req.Status
			if ok {
				*response = "SUCCESS " + server.Name + "." + args.AccountName + " " + obj.AccountBalance
			} else {
				*response = "ABORT"
			}
		}
	} else {
		// Has both readers and writers at the same time (Technically should never happen)
		if obj.Readers.Size() != 0 {
			fmt.Println("Reader-Writer Conflict!")
		}
		obj.AccountLock.Unlock()
		return errors.New("Read: Account accountName=" + args.AccountName + ", Transaction=" + args.TransactionID + ". Reader-writer conflict.")
	}

	return nil
}

// coordinator
type Coordinator struct {
	ClientGraph       *graph.Graph
	TransactionBuffer map[string]graph.Node
}

func NewCoordinator() *Coordinator {
	coordinator := new(Coordinator)
	coordinator.ClientGraph = graph.New(graph.Directed)
	coordinator.TransactionBuffer = make(map[string]graph.Node)
	return coordinator
}

func (coordinator *Coordinator) NewTransaction(args *CoordinatorArgs, response *string) error {
	_, found := coordinator.TransactionBuffer[args.ORIGIN]
	if !found {
		fmt.Println("Adding new transaction " + args.ORIGIN)
		coordinator.TransactionBuffer[args.ORIGIN] = coordinator.ClientGraph.MakeNode()
	}
	return nil
}

func (coordinator *Coordinator) RemoveTransaction(args *CoordinatorArgs, response *string) error {
	node, found := coordinator.TransactionBuffer[args.ORIGIN]
	if !found {
		return errors.New("RemoveTransaction: Transaction not found in coordinator")
	} else {
		fmt.Println("Removing transaction " + args.ORIGIN)
		delete(coordinator.TransactionBuffer, args.ORIGIN)
		coordinator.ClientGraph.RemoveNode(&node)
	}
	return nil
}

func InitializeCoordinator(port string) {
	coordinator := NewCoordinator()
	rpc.Register(coordinator)

	tcpAddress, err := net.ResolveTCPAddr("tcp", ":"+port)
	if err != nil {
		fmt.Println("ResolveTCPAddr Error")
		fmt.Println(err)
		os.Exit(1)
	}

	listener, err := net.ListenTCP("tcp", tcpAddress)
	if err != nil {
		fmt.Println("ListenTCP Error")
		fmt.Println(err)
		os.Exit(1)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Accept Error")
			continue
		}
		go rpc.ServeConn(conn)
	}
}

// client
var ServerMap = map[string]string{
	"A": "sp20-cs425-g35-01.cs.illinois.edu:8000",
	"B": "sp20-cs425-g35-01.cs.illinois.edu:8001",
	"C": "sp20-cs425-g35-01.cs.illinois.edu:8002",
	"D": "sp20-cs425-g35-01.cs.illinois.edu:8003",
	"E": "sp20-cs425-g35-01.cs.illinois.edu:8004"}

func InitializeClient(name string) {
	// intialize all the local variables
	clienttempstore = make(map[string]map[string]string)
	clientid = GetLocalIP() + name
	clientworking = false
	clientIsAborted = false
	clientCommandsqueue = NewQueue()
	clientReadLockSet = NewSet()
	totaltransactions = 1
	// start reading commmands
	in := bufio.NewReader(os.Stdin)
	go func() {
		for {
			command, _, err := in.ReadLine()
			if err != nil {
				fmt.Println("Unable to Read line: ")
				fmt.Println(err)
				os.Exit(1)
			}
			// if it is abort, directly go to abort
			if string(command) == "ABORT" {
				BEGIN_bool = false
				start = time.Now()
				handleAbort(false)
			} else {
				clientCommandsqueue.Push(string(command))
			}
		}
	}()
	for {
		if !clientCommandsqueue.IsEmpty() {
			currentCommand := clientCommandsqueue.Pop()
			command := strings.Split(currentCommand, " ")
			action := command[0]
			if action == "BEGIN" {
				BEGIN_bool = true
				start = time.Now()
				fmt.Println("BEGING_BOOL:")
				fmt.Println(BEGIN_bool)
				handleBegin()
				go countdown()
			} else if action == "DEPOSIT" {
				start = time.Now()
				handleDeposit(command)
			} else if action == "BALANCE" {
				start = time.Now()
				handleBalance(command)
			} else if action == "COMMIT" {
				start = time.Now()
				BEGIN_bool = false
				handleCommit()
			} else if action == "WITHDRAW" {
				start = time.Now()
				handleWithdraw(command)
			} else {
				fmt.Println("command: " + currentCommand + " is invalid")
			}
		} else {
			// stop the for loop and wait for new command
			<-clientCommandsqueue.NotifyChann
		}
	}
}

func countdown() {
	for BEGIN_bool == true {
		elapsed := time.Since(start)
		// fmt.Println("elapsed:")
		// fmt.Println(elapsed)
		if elapsed > timeout {
			fmt.Println("1 minute passed without receive any commands, going to abort")
			BEGIN_bool = false
			handleAbort(false)
		}
	}
}

func handleBegin() {
	clientlock.Lock()
	clientworking = true
	clientIsAborted = false
	transactionID := clientid + strconv.Itoa(totaltransactions)
	clientlock.Unlock()
	RPCToCoor("BEGIN", transactionID, []string{})
	fmt.Println("OK")
}

func handleDeposit(command []string) {
	clientlock.RLock()
	if clientIsAborted {
		fmt.Println("No Transaction is processing")
		clientlock.RUnlock()
		return
	}
	clientlock.RUnlock()
	if len(command) != 3 {
		fmt.Println("Invalid command")
	} else if !clientworking {
		fmt.Println("No Transaction is processing")
	} else {
		server := strings.Split(command[1], ".")[0]
		accountName := strings.Split(command[1], ".")[1]
		value := command[2]
		if _, exist := ServerMap[server]; !exist {
			fmt.Println("Server does not exist")
			return
		}
		if _, exist := clienttempstore[server]; !exist {
			clienttempstore[server] = make(map[string]string)
		}
		value_convert, _ := strconv.Atoi(value)
		if value_convert < 0 {
			fmt.Println("You cannot deposit negtive money")
			handleAbort(false)
		}
		if v, exist := clienttempstore[server][accountName]; exist && v != "" {
			balance, _ := strconv.Atoi(clienttempstore[server][accountName])
			value_convert, _ := strconv.Atoi(value)
			balance = balance + value_convert
			clienttempstore[server][accountName] = strconv.Itoa(balance)
			fmt.Println("OK")
		} else {
			transactionID := clientid + strconv.Itoa(totaltransactions)
			fmt.Println("Deposit information args:")
			fmt.Println(server, accountName, value, transactionID)
			response := RPCToServer("WriteLockRequest", server, accountName, value, transactionID)
			fmt.Println(response)
			if strings.HasPrefix(response, "SUCCESS") {
				content := strings.Split(response, " ")
				balance, _ := strconv.Atoi(content[2])
				value_convert, _ := strconv.Atoi(value)
				balance = balance + value_convert
				clienttempstore[server][accountName] = strconv.Itoa(balance)
				fmt.Println("OK")
			} else if strings.HasPrefix(response, "ABORT") {
				handleAbort(false)
			} else if strings.HasPrefix(response, "NOT FOUND") {
				clienttempstore[server][accountName] = value
				fmt.Println("OK")
			}
		}
	}
}

func handleBalance(command []string) {
	clientlock.RLock()
	if clientIsAborted {
		fmt.Println("No Transaction is processing")
		clientlock.RUnlock()
		return
	}
	clientlock.RUnlock()
	if len(command) != 2 {
		fmt.Println("Invalid command")
	} else if !clientworking {
		fmt.Println("No Transaction is processing")
	} else {
		server := strings.Split(command[1], ".")[0]
		accountName := strings.Split(command[1], ".")[1]
		if _, exist := ServerMap[server]; !exist {
			fmt.Println("Server does not exist")
			return
		}
		if v, exist := clienttempstore[server][accountName]; exist && v != "" {
			fmt.Println(command[1] + " " + "=" + " " + v)
		} else {
			clientReadLockSet.SetAdd(server + "." + accountName)
			transactionID := clientid + strconv.Itoa(totaltransactions)
			response := RPCToServer("ReadLockRequest", server, accountName, "", transactionID)
			if strings.HasPrefix(response, "SUCCESS") {
				clientReadLockSet.SetAdd(server + "." + accountName)
				content := strings.Split(response, " ")
				fmt.Println(command[1] + " " + "=" + " " + content[2])
			} else if strings.HasPrefix(response, "ABORT") {
				handleAbort(false)
			} else if strings.HasPrefix(response, "NOT FOUND") {
				fmt.Println("NOT FOUND")
				handleAbort(true)
			}
		}
	}
}

func handleWithdraw(command []string) {
	clientlock.RLock()
	if clientIsAborted {
		fmt.Println("No Transaction is processing")
		clientlock.RUnlock()
		return
	}
	clientlock.RUnlock()
	if len(command) != 3 {
		fmt.Println("Invalid command")
	} else if !clientworking {
		fmt.Println("No Transaction is processing")
	} else {
		server := strings.Split(command[1], ".")[0]
		accountName := strings.Split(command[1], ".")[1]
		value := command[2]
		if _, exist := ServerMap[server]; !exist {
			fmt.Println("Server does not exist")
			return
		}
		if _, exist := clienttempstore[server]; !exist {
			clienttempstore[server] = make(map[string]string)
		}
		value_convert, _ := strconv.Atoi(value)
		if value_convert < 0 {
			fmt.Println("You cannot withdraw negtive money")
			handleAbort(false)
		}
		if v, exist := clienttempstore[server][accountName]; exist && v != "" {
			balance, _ := strconv.Atoi(clienttempstore[server][accountName])
			value_convert, _ := strconv.Atoi(value)
			balance = balance - value_convert
			clienttempstore[server][accountName] = strconv.Itoa(balance)
			fmt.Println("OK")
		} else {
			clientReadLockSet.SetAdd(server + "." + accountName)
			transactionID := clientid + strconv.Itoa(totaltransactions)
			response := RPCToServer("WriteLockRequest", server, accountName, value, transactionID)
			if strings.HasPrefix(response, "SUCCESS") {
				content := strings.Split(response, " ")
				balance, _ := strconv.Atoi(content[2])
				value_convert, _ := strconv.Atoi(value)
				balance = balance - value_convert
				clienttempstore[server][accountName] = strconv.Itoa(balance)
				fmt.Println("OK")
			} else if strings.HasPrefix(response, "ABORT") {
				handleAbort(false)
			} else if strings.HasPrefix(response, "NOT FOUND") {
				fmt.Println("NOT FOUND")
				handleAbort(true)
			}
		}
	}
}

func handleCommit() {
	clientlock.RLock()
	if clientIsAborted {
		fmt.Println("No Transaction is processing")
		clientlock.RUnlock()
		return
	}
	clientlock.RUnlock()

	clientworking = false
	check := false
	for _, storage := range clienttempstore {
		for _, value := range storage {
			value_convert, _ := strconv.Atoi(value)
			if value_convert < 0 {
				check = true
				handleAbort(false)
			}
		}
	}
	if !check {
		for server, storage := range clienttempstore {
			for accountName, value := range storage {
				transactionID := clientid + strconv.Itoa(totaltransactions)
				RPCToServer("Change", server, accountName, value, transactionID)
				RPCToServer("UpdateAccount", server, accountName, "", transactionID)
			}
		}
		Cleanup()
		totaltransactions++
		fmt.Println("COMMIT OK")
	}
}

func handleAbort(Notprint bool) {
	clientlock.RLock()
	if clientIsAborted {
		clientlock.RUnlock()
		return
	}
	clientlock.RUnlock()
	clientlock.Lock()
	clientworking = false
	clientIsAborted = true
	clientlock.Unlock()
	for server, storage := range clienttempstore {
		for accountName, _ := range storage {
			transactionID := clientid + strconv.Itoa(totaltransactions)
			RPCToServer("UpdateAccount", server, accountName, "", transactionID)
		}
	}
	Cleanup()
	if !Notprint {
		fmt.Println("ABORTED")
	}
}

//Cleanup ... Cleanup temp container for local storage
func Cleanup() {
	transactionID := clientid + strconv.Itoa(totaltransactions)
	for _, lockInfo := range clientReadLockSet.SetToArray() {
		server := strings.Split(lockInfo, ".")[0]
		accountName := strings.Split(lockInfo, ".")[1]
		RPCToServer("UpdateAccount", server, accountName, "", transactionID)
	}
	clienttempstore = make(map[string]map[string]string)
	clientReadLockSet = NewSet()
	clientCommandsqueue.Clear()
	RPCToCoor("REMOVE", transactionID, []string{})
}

//RPCToServer ... Remote Procedure Call to Server
func RPCToServer(action string, server string, accountName string, value string, transactionID string) string {
	serverAddrr := ServerMap[server]
	rpcClient, err := rpc.Dial("tcp", serverAddrr)
	if err != nil {
		fmt.Println("Unable to Dial")
		fmt.Println(err)
		os.Exit(1)
	}
	args := &Args{AccountName: accountName, Value: value, TransactionID: transactionID}
	var response string
	if action == "WriteLockRequest" {
		err = rpcClient.Call("Server.WriteLockRequest", args, &response)
	} else if action == "Change" {
		err = rpcClient.Call("Server.Change", args, &response)
	} else if action == "ReadLockRequest" {
		err = rpcClient.Call("Server.ReadLockRequest", args, &response)
	} else if action == "UpdateAccount" {
		err = rpcClient.Call("Server.UpdateAccount", args, &response)
	} else {
		fmt.Println("Unknown Action is used " + action)
	}
	if err != nil {
		fmt.Println("Unable to Request")
		fmt.Println(err)
		os.Exit(1)
	}
	rpcClient.Close()
	return response
}

func main() {
	args := os.Args
	if len(args) <= 2 {
		fmt.Println("Please specify server/coordinator/client for the program ")
		os.Exit(1)
	}
	switch args[1] {
	case "server":
		if len(args) != 4 {
			fmt.Println("Usage error: ./mp3 server name port")
			os.Exit(1)
		}
		InitializeServer(args[2], args[3])
	case "client":
		if len(args) != 3 {
			fmt.Println("Usage error: ./mp3 client name")
			os.Exit(1)
		}
		InitializeClient(args[2])
	case "coordinator":
		if len(args) != 3 {
			fmt.Println("Usage error: ./mp3 coordinator port")
			os.Exit(1)
		}
		InitializeCoordinator(args[2])
	default:
		fmt.Println("Usage error: Unknow type")
		os.Exit(1)
	}
}
