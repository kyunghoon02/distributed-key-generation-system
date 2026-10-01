package main

import (
	"errors"
	"flag"
	"log/slog"
	"net"
	"os"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/participant"
)

func runRealParticipant(args []string) error {
	flags := flag.NewFlagSet("real-participant", flag.ContinueOnError)
	id := flags.String("id", "", "participant ID")
	index := flags.Uint("index", 0, "participant index")
	address := flags.String("listen", "127.0.0.1:0", "TCP listen address")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *id == "" || *index > uint(^uint32(0)) {
		return errors.New("valid -id and -index are required")
	}
	host, _, err := net.SplitHostPort(*address)
	if err != nil {
		return err
	}
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return errors.New("real DKG participant must listen on loopback")
		}
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return err
	}
	defer listener.Close()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	server, err := participant.NewRealServer(*id, uint32(*index), logger)
	if err != nil {
		return err
	}
	return server.Serve(listener)
}
