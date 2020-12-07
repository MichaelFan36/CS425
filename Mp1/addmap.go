package addmap

import (
	"strconv"
	"strings"
)

//package main

func UpdateBalance(msg string, Balance map[string]int) {
	//fmt.Println("msg_Balance: " + msg)
	sList := strings.Split(msg, " ")
	Behavior := sList[0]
	if Behavior == "DEPOSIT" {
		name := sList[1]
		Money := sList[2]
		Money_str := strings.Split(Money, "\n")
		Money_txt := Money_str[0]
		CMoney, _ := strconv.ParseFloat(Money_txt, 64)
		if _, ok := Balance[name]; !ok {
			Balance[name] = int(CMoney)
		} else {
			Balance[name] += int(CMoney)
		}
	} else if Behavior == "TRANSFER" {
		sender := sList[1]
		receiver := sList[3]
		Money := sList[4]
		Money_str := strings.Split(Money, "\n")
		Money_txt := Money_str[0]
		CMoney, _ := strconv.ParseFloat(Money_txt, 64)
		if _, ok := Balance[sender]; !ok {
			Balance[sender] = 0
		} else if Balance[sender] >= int(CMoney) {
			Balance[sender] -= int(CMoney)
			Balance[receiver] += int(CMoney)
		}
	}

}

func AddProposeMap(m map[string][10]int, event_id string, n int) {
	var temp_arr [10]int
	for i := 0; i < len(m[event_id]); i++ {
		if m[event_id][i] == 0 {
			temp_arr = m[event_id]
			if contains(temp_arr, n) {
				break
			}
			temp_arr[i] = n
			m[event_id] = temp_arr
			break
		}
	}
}

func contains(arr [10]int, n int) bool {
	for _, a := range arr {
		if a == n {
			return true
		}
	}
	return false
}

// func add(m map[string]map[string]int, path, country string) {
//     mm, ok := m[path]
//     if !ok {
//         mm = make(map[string]int)
//         m[path] = mm
//     }
//     mm[country]++
// }

// func main() {
// 	// hits := make(map[string]map[string]int)
// 	// var hits = make(map[string]map[string]int)
// 	var ProposeMap = make(map[float64][10]int)

// 	// add(hits, "/doc/", "au")
// 	// n := hits["/doc/"]["au"]
// 	// n_str := strconv.Itoa(n)
// 	// fmt.Print(string(n_str) +"\n")

// 	ProposeMap[12.3] = [10]int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
// 	AddProposeMap(ProposeMap, 3.1, 1)
// 	AddProposeMap(ProposeMap, 3.1, 1)
// 	AddProposeMap(ProposeMap, 3.1, 3)
// 	AddProposeMap(ProposeMap, 3.1, 2)
// 	AddProposeMap(ProposeMap, 3.1, 3)

// 	for k, v := range ProposeMap {
// 		fmt.Println("k:", k, "v:", v)
// 	}

// }
