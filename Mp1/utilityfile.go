package utilityfile

import (
	"fmt"
	"log"
	"os"
	"time"
)

var currtime = ""

func CreateFile() {
	currtime = time.Now().Format("2006.01.02 15:04:05")
	path := currtime + "_logger_time.txt"

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

func WriteFile(text string) {
	//currtime = time.Now().Format("2006.01.02 15:04:05")
	path := currtime + "_logger_time.txt"
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

func AppendFile(text string) {
	//currtime := time.Now().Format("2006.01.02 15:04:05")
	path := currtime + "_logger_time.txt"
	var file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}
	defer file.Close()
	if _, err := file.WriteString(text); err != nil {
		log.Println(err)
	}
}
