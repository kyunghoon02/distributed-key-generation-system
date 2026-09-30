package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/participant"
)

func main() {
	id := flag.String("id", "", "stable participant ID")
	listenAddress := flag.String("listen", "127.0.0.1:0", "TCP listen address")
	flag.Parse()
	if *id == "" {
		log.Fatal("-id is required")
	}
	listener, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	fmt.Printf("participant %s listening on %s\n", *id, listener.Addr())
	if err := participant.NewServer(*id, cryptoadapter.Mock{}).Serve(listener); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
