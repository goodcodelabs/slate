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

	send("health")
	send("add_workspace foo")
	send("add_catalog bar")
}
