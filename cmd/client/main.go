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
	defer func() {
		if err := conn.Close(); err != nil {
			panic(err)
		}
	}()

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
	send("add_catalog baz")
	send("ls_catalogs")
	send("add_catalog bunz")
	send("del_catalog bar")
	send("ls_catalogs")
}
