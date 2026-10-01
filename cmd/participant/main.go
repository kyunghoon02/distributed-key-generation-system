package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/participant"
)

func main() {
	id := flag.String("id", "", "stable participant ID")
	listenAddress := flag.String("listen", "127.0.0.1:0", "TCP listen address")
	stateFile := flag.String("state-file", "", "durable participant state file")
	metricsAddress := flag.String("metrics-listen", "", "optional Prometheus HTTP listen address")
	flag.Parse()
	if *id == "" {
		log.Fatal("-id is required")
	}
	listener, err := net.Listen("tcp", *listenAddress)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	fmt.Printf("participant %s listening on %s\n", *id, listener.Addr())
	server := participant.NewServer(*id, cryptoadapter.Mock{})
	if *stateFile != "" {
		server, err = participant.NewDurableServer(*id, cryptoadapter.Mock{}, *stateFile)
		if err != nil {
			log.Fatalf("recover participant state: %v", err)
		}
	}
	defer server.Close()
	server.SetLogger(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if *metricsAddress != "" {
		metricsListener, err := net.Listen("tcp", *metricsAddress)
		if err != nil {
			log.Fatalf("metrics listen: %v", err)
		}
		defer metricsListener.Close()
		mux := http.NewServeMux()
		mux.Handle("/metrics", server.MetricsHandler())
		go func() { _ = http.Serve(metricsListener, mux) }()
	}
	if err := server.Serve(listener); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
