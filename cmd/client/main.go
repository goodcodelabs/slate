package main

import (
	"bufio"
	"fmt"
	"net"
)

func main() {
	conn, err := net.Dial("tcp", "localhost:4242")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)

	send := func(msg string) {
		// server expects newline-terminated messages
		if _, err := conn.Write([]byte(msg + "\n")); err != nil {
			panic(err)
		}
		// server echoes back "Received: <msg>\n"
		line, err := reader.ReadString('\n')
		if err != nil {
			panic(err)
		}
		fmt.Print("Server replied: ", line)
	}

	/// create 1MM random string based keys and values
	//for i := 0; i < 1000000; i++ {
	//	send(fmt.Sprintf("set %s %s", ksuid.New().String(), ksuid.New().String()))
	//}

	send("set foo bar")
	send("set bar foo")
	send("get foo")
	send("get blue")
	send("dawg")
	send("get bar")
	send("del bar")
	send("get bar")
	send("del dawg")
	send("start_session")
	send("health")
}
