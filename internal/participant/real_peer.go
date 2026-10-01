package participant

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/transport"
)

// validatePeers is deliberately local-only until address provisioning and
// multi-host enrollment have a separate operational design.
func (s *RealServer) validatePeers(config *api.RealConfig) error {
	self, err := s.node.Identity()
	if err != nil {
		return err
	}
	if len(config.Identities) != 4 || config.Threshold != 3 || len(config.Peers) != 3 {
		return errors.New("peer mode requires a 3-of-4 roster")
	}
	known := make(map[string]bool, 4)
	for _, identity := range config.Identities {
		known[identity.ID] = true
	}
	if !known[self.ID] || config.Peers[self.ID] != "" {
		return errors.New("invalid local peer roster")
	}
	addresses := make(map[string]bool, 3)
	for id, address := range config.Peers {
		if !known[id] || id == self.ID || address == "" || addresses[address] {
			return errors.New("invalid peer identity or duplicate address")
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return errors.New("peer mode currently requires loopback addresses")
		}
		addresses[address] = true
	}
	return nil
}

func (s *RealServer) runPeerCeremony() {
	err := s.peerCeremony()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peerRunning = false
	if err != nil {
		s.peerError = err.Error()
		if s.node.Stage() != "FINALIZE" {
			_ = s.node.Abort()
		}
	}
}

func (s *RealServer) peerCeremony() error {
	s.mu.Lock()
	deal, err := s.node.Deals()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := s.broadcastPeerPacket(deal); err != nil {
		return err
	}
	if err := s.waitForPeerPackets("deal"); err != nil {
		return err
	}
	s.mu.Lock()
	response, err := s.node.ProcessDeals()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if response != nil {
		if err := s.broadcastPeerPacket(*response); err != nil {
			return err
		}
	}
	if err := s.waitForPeerPackets("response"); err != nil {
		return err
	}
	s.mu.Lock()
	justification, err := s.node.ProcessResponses()
	stage := s.node.Stage()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if stage == "FINALIZE" {
		return nil
	}
	if justification != nil {
		if err := s.broadcastPeerPacket(*justification); err != nil {
			return err
		}
	}
	// The direct E0 path requires a complaint-free ceremony. A complaint needs
	// a separate justification completion rule before we can claim finality.
	return errors.New("peer ceremony needs justification handling")
}

func (s *RealServer) waitForPeerPackets(kind string) error {
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		count := s.node.Received(kind)
		s.mu.Unlock()
		if count == len(s.peers) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s packets", kind)
}

func (s *RealServer) broadcastPeerPacket(packet cryptoadapter.KyberPacket) error {
	for id, address := range s.peers {
		config := s.peerTLS.Clone()
		config.ServerName = id
		client := transport.TCP{Timeout: time.Second, TLSConfig: config}
		deadline := time.Now().Add(8 * time.Second)
		var lastErr error
		for time.Now().Before(deadline) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_, lastErr = client.Call(ctx, address, api.Request{Operation: "real-accept", RealPacket: &packet})
			cancel()
			if lastErr == nil {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if lastErr != nil {
			return fmt.Errorf("send %s from %s to %s: %w", packet.Kind, packet.From, id, lastErr)
		}
	}
	return nil
}
