package main

import (
	"addmap"
	"bufio"
	"container/heap"
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"utilityfile"
)

var ip_array = [10]string{
	"172.22.158.115",
	"172.22.94.115",
	"172.22.156.116",
	"172.22.158.116",
	"172.22.94.116",
	"172.22.156.117",
	"172.22.158.117",
	"172.22.94.117",
	"172.22.156.118",
	"172.22.158.118"}

var count = 0 //for channel connection
var eventnumber = 0
var id = 1

//ID for TCP (only node1)
var ID = 0

var rsenderid = 0

const maxNodes int = 10

//var conn net.Conn
var c net.Conn

// conn used to check if all atms are connected
var conn [maxNodes]net.Conn

var isalive [maxNodes]bool

var allproposed bool
var totalatms = 0
var totalconn = 0
var msg = ""
var EventPriority = 0.0
var length = 0
var interrupted bool = false
var iter = 0 //common iterator

// ProposeMap used to store eventid->[node numbers]
var ProposeMap = make(map[string][10]int)

// EventMap used to store Eventid->Eventnumber (not eventnumber but index of the node )
var EventMap = make(map[string]int)

// EventidMap used to store Eventnumber->Eventid (not eventnumber but index of the node )->Eventid
var EventidMap = make(map[int]string)

// Balance used to store balance   string(username)->balance
var Balance = make(map[string]int)

// Messagemap used to store messages  eventid->msg
var Messagemap = make(map[string]string)
var mtx sync.Mutex
var filelock sync.Mutex

var pq = make(PriorityQueue, 0)

func gettimestring() string {
	time := fmt.Sprintf("%f", float64(time.Now().UnixNano())/float64(time.Second))
	return time
}

func PrintBalance() {
	for !interrupted {
		fmt.Println("Balance info: ")
		for k, v := range Balance {
			fmt.Println("Username: ", k, "Balance: ", v)
		}
		duration := time.Duration(5) * time.Second
		time.Sleep(duration)

	}
}

//credit: https://golang.org/pkg/container/heap/
// An Item is something we manage in a priority queue.
type Item struct {
	value      bool    // The value of the item; arbitrary.
	priority   float64 // The priority of the item in the queue.
	generateid int     //The process id of the original sender
	// The index is needed by update and is maintained by the heap.Interface methods.
	index int // The index of the item in the heap.
}

// A PriorityQueue implements heap.Interface and holds Items.
type PriorityQueue []*Item

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.

	//Original
	return pq[i].priority < pq[j].priority
	// we want Pop to give us the lowest (priority)
	// return pq[i].priority < pq[j].priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]

	temp_i := EventidMap[i]
	temp_j := EventidMap[j]
	EventidMap[i], EventidMap[j] = EventidMap[j], EventidMap[i]
	EventMap[temp_i], EventMap[temp_j] = EventMap[temp_j], EventMap[temp_i]

	pq[i].index = i
	pq[j].index = j
}

// Push into pq
func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

// Pop from pq
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// update modifies the priority and value of an Item in the queue.
func (pq *PriorityQueue) Update(item *Item, value bool, priority float64) {
	item.value = value
	if priority >= item.priority {
		item.priority = priority
		eventnumber = int(math.Max(float64(eventnumber+1), float64(int(priority)+1)))
	}
	heap.Fix(pq, item.index)
}

func SendPropose(message string, c net.Conn) {
	// fmt.Println("isalive array", isalive)
	if isalive[rsenderid] == true {
		// filelock.Lock()
		// message_str := strings.Split(message, "\n")
		// message_txt := message_str[0]
		// utilityfile.AppendFile(message_txt + "\n")
		// bandwidth := fmt.Sprintf("%d", len(message)) + "\n"
		// utilityfile.AppendFile(bandwidth)
		// filelock.Unlock()
		fmt.Fprintf(c, message+"\n")
	}
}

func Multicast(message string) {
	for i := 0; i < totalconn; i++ {
		if i == ID {
			continue
		}
		// filelock.Lock()
		// message_str := strings.Split(message, "\n")
		// message_txt := message_str[0]
		// utilityfile.AppendFile(message_txt + "\n")
		// bandwidth := fmt.Sprintf("%d", len(message)) + "\n"
		// utilityfile.AppendFile(bandwidth)
		// filelock.Unlock()
		fmt.Fprintf(conn[i], message+"\n")
	}
	// fmt.Println("message in multicast: " + message)
}

//Receiver
func handleConnection(c net.Conn) {
	reader := bufio.NewReader(c)
	for !interrupted {

		netData, err := reader.ReadString('\n')
		if err != nil {
			// time := fmt.Sprintf("%f", float64(time.Now().UnixNano())/float64(time.Second))
			// fmt.Println(err)
			break
		}
		mtx.Lock()
		// fmt.Println("Netdata:", netData)
		if netData == "" || netData == "\n" {
			mtx.Unlock()
			continue
		}
		if netData[len(netData)-1] == '\n' {
			netData = netData[0 : len(netData)-1]
		}
		sList := strings.Split(netData, " ")
		msgtype := sList[0]

		// filelock.Lock()
		// netdata_str := strings.Split(netData, "\n")
		// netdata_txt := netdata_str[0]
		// utilityfile.AppendFile(netdata_txt + "\n")
		// bandwidth := fmt.Sprintf("%d", len(netData)) + "\n"
		// utilityfile.AppendFile(bandwidth)
		// filelock.Unlock()

		if msgtype == "Request" {

			ReceivedEventid := sList[1] + " " + sList[2]
			msg = strings.SplitN(netData, " ", 6)[5]
			EventMap[ReceivedEventid] = eventnumber
			EventidMap[eventnumber] = ReceivedEventid
			eventnumber = eventnumber + 1

			Messagemap[ReceivedEventid] = msg
			EventPriority = float64(eventnumber) + float64(id)*0.1
			EventPriority_Str := sList[3]
			EventPriority2, _ := strconv.ParseFloat(EventPriority_Str, 64)
			senderid, _ := strconv.Atoi(sList[4])
			rsenderid = senderid - 1
			if EventPriority2 > EventPriority {
				EventPriority = EventPriority2
			}
			item := &Item{
				value:      false,
				priority:   EventPriority,
				generateid: senderid - 1,
				index:      eventnumber,
			}
			heap.Push(&pq, item)
			EventMap[ReceivedEventid] = item.index
			EventidMap[EventMap[ReceivedEventid]] = ReceivedEventid
			SendPropose("Propose"+" "+ReceivedEventid+" "+fmt.Sprintf("%f", EventPriority)+" "+strconv.Itoa(id)+" "+msg, c)
		}

		if msgtype == "Propose" {
			ReceivedEventid := sList[1] + " " + sList[2]
			msg = strings.SplitN(netData, " ", 6)[5]
			EventPriority_Str := sList[3]
			index := EventMap[ReceivedEventid]
			EventPriority, _ := strconv.ParseFloat(EventPriority_Str, 64)
			pq.Update(pq[index], false, EventPriority)

			senderid, _ := strconv.Atoi(sList[4])

			addmap.AddProposeMap(ProposeMap, ReceivedEventid, senderid)

			allproposed = true
			for i := 0; i < totalatms; i++ {
				if ProposeMap[ReceivedEventid][i] == 0 {
					allproposed = false
				}
			}
			if allproposed == true {

				index := EventMap[ReceivedEventid]
				item := pq[index]
				pq[index].value = true
				EventPriority := item.priority
				Multicast("Agreed" + " " + ReceivedEventid + " " + fmt.Sprintf("%f", EventPriority) + " " + strconv.Itoa(id) + " " + msg)
				length = pq.Len()
				for pq.Len() != 0 {
					if pq[0].value == true && isalive[pq[0].generateid] != false {
						eventid := EventidMap[pq[0].index]
						msg = Messagemap[eventid]

						if netData[len(netData)-1] == '\n' {
							netData = netData[0 : len(netData)-1]
						}

						// fmt.Println("msg sent into Balance is", msg)
						addmap.UpdateBalance(msg, Balance)

						time := gettimestring()
						filelock.Lock()
						msg_str := strings.Split(msg, "\n")
						msg_txt := msg_str[0]
						utilityfile.AppendFile(time + " Lastmessage " + eventid + " " + msg_txt + "\n")
						filelock.Unlock()

						heap.Pop(&pq)
						length = pq.Len()
						if length == 0 {
							break
						}

					} else if isalive[pq[0].generateid] == false {
						heap.Pop(&pq)
					} else if pq[0].value == false && isalive[pq[0].generateid] != false {
						allproposed = true
						recent_id := EventidMap[pq[0].index]
						for i := 0; i < totalatms; i++ {
							if ProposeMap[recent_id][i] == 0 {
								allproposed = false
							}
						}
						if allproposed == true {
							pq[0].value = true
						} else {
							break
						}
					}
				}
			}
		}
		if msgtype == "Agreed" {
			ReceivedEventid := sList[1] + " " + sList[2]
			msg = strings.SplitN(netData, " ", 6)[5]
			EventPriority_Str := sList[3]
			index := EventMap[ReceivedEventid]
			EventPriority, _ := strconv.ParseFloat(EventPriority_Str, 64)
			pq.Update(pq[index], true, EventPriority)
			for pq.Len() != 0 {
				if pq[0].value == true && isalive[pq[0].generateid] != false {
					eventid := EventidMap[pq[0].index]
					msg = Messagemap[eventid]
					// fmt.Println("msg being updated in Agree is", msg)
					addmap.UpdateBalance(msg, Balance)

					time := gettimestring()
					filelock.Lock()
					msg_str := strings.Split(msg, "\n")
					msg_txt := msg_str[0]
					utilityfile.AppendFile(time + " Lastmessage " + eventid + " " + msg_txt + "\n")
					filelock.Unlock()

					heap.Pop(&pq)
				} else if isalive[pq[0].generateid] == false {
					heap.Pop(&pq)
				} else if pq[0].value == false && isalive[pq[0].generateid] != false {
					allproposed = true
					recent_id := EventidMap[pq[0].index]
					for i := 0; i < totalatms; i++ {
						if ProposeMap[recent_id][i] == 0 {
							allproposed = false
						}
					}
					if allproposed == true {
						pq[0].value = true
					} else {
						break
					}
				}
			}
		}

		matched, err := regexp.MatchString(`\d`, msgtype)
		if matched == true {
			target_id, _ := strconv.Atoi(msgtype)
			isalive[target_id] = false
			totalatms -= 1
			for pq.Len() != 0 {
				if pq[0].generateid == target_id {
					heap.Pop(&pq)
				}
			}
		}

		mtx.Unlock()
		// timestamps, err := strconv.ParseFloat(fmt.Sprintf("%f", float64(time.Now().UnixNano())/float64(time.Second)), 64)
		// timestampc, err := strconv.ParseFloat(strings.Split(netData, " ")[0], 64)
		// timedelay := fmt.Sprintf("%f", timestamps-timestampc) + "\n"
		// bandwidth := fmt.Sprintf("%d", len(netData)) + "\n"
		// // fmt.Println("Time Delay:", timestamps-timestampc)
		// // fmt.Println("bandwidth is ", len(netData))
		// fmt.Print(netData)
		// mtx.Lock()
		// WriteFile(netData)
		// WriteFile(timedelay)
		// WriteFile(bandwidth)
		// mtx.Unlock()

	}
	c.Close()
}

func main() {

	utilityfile.CreateFile()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT)
	go func() {
		_ = <-sigs
		interrupted = true
		Multicast(strconv.Itoa(ID) + " crashed")
	}()

	// Some items and their priorities.
	// Create a priority queue, put the items in it, and
	// establish the priority queue (heap) invariants.
	heap.Init(&pq)

	//sender
	arguments := os.Args
	if len(arguments) == 1 {
		fmt.Println("Please provide host:port.")
		return
	}
	totalatms, _ = strconv.Atoi(arguments[1])
	totalconn = totalatms
	for i := 0; i < totalatms; i++ {
		isalive[i] = true
	}
	// server
	port := arguments[2]
	l, err := net.Listen("tcp", ":"+port)
	if err != nil {
		// // fmt.Println(err)
		return
	}
	for i := ID + 1; i < totalatms; i++ {
		conn[i], err = l.Accept()
		if err != nil {
			// // fmt.Println(err)
			return
		}
		count++
		go handleConnection(conn[i])
	}

	defer l.Close()

	for i := 0; i < ID; i++ {
		for {
			conn[i], err = net.Dial("tcp", ip_array[i]+":"+port)
			if err == nil {
				break
			}
		}
		go handleConnection(conn[i])
		count++
	}

	go PrintBalance()
	for !interrupted {

		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')

		if count >= (totalatms - 1) {
			mtx.Lock()
			if text == "" || text == "\n" {
				mtx.Unlock()
				continue
			}
			eventnumber = eventnumber + 1
			EventPriority = float64(eventnumber) + float64(id)*0.1
			Eventid := strconv.Itoa(eventnumber) + " " + strconv.Itoa(id)
			Messagemap[Eventid] = text
			EventMap[Eventid] = eventnumber - 1
			EventidMap[eventnumber-1] = Eventid

			item := &Item{
				value:      false,
				priority:   EventPriority,
				generateid: ID,
				index:      eventnumber,
			}
			heap.Push(&pq, item)
			EventMap[Eventid] = item.index
			EventidMap[EventMap[Eventid]] = Eventid
			addmap.AddProposeMap(ProposeMap, Eventid, id)

			time := gettimestring()
			filelock.Lock()
			text_str := strings.Split(text, "\n")
			text_txt := text_str[0]
			utilityfile.AppendFile(time + " Firstmessage " + Eventid + " " + text_txt + "\n")
			filelock.Unlock()

			if totalatms == 1 {
				addmap.UpdateBalance(text, Balance)
				heap.Pop(&pq)
			} else {
				message := "Request" + " " + Eventid + " " + fmt.Sprintf("%f", EventPriority) + " " + strconv.Itoa(id) + " " + text
				Multicast(message)
			}
			mtx.Unlock()

		}
	}

}
