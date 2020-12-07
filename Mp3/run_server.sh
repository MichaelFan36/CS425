go build group35_mp3.go
sleep 1
./group35_mp3 server A 8000 &
./group35_mp3 server B 8001 &
./group35_mp3 server C 8002 &
./group35_mp3 server D 8003 &
./group35_mp3 server E 8004 
sleep 5000