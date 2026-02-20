package p2p

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	libp2p "github.com/libp2p/go-libp2p"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// --- Test helpers ---

func makeTestKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	return pub, priv
}

func makeTestProposal(t *testing.T) *types.Proposal {
	t.Helper()
	pub, priv := makeTestKeypair(t)
	addr := crypto.AddressFromPubKey(pub)

	block := &types.Block{
		Header: types.BlockHeader{
			Height:     1,
			Round:      0,
			ProposerID: addr,
			ChainID:    []byte("test-chain"),
			BlockTime:  uint64(time.Now().UnixNano()),
		},
		Transactions: [][]byte{[]byte("tx1"), []byte("tx2")},
	}
	block.Header.BlockHash = block.Header.ComputeHash()

	proposal := &types.Proposal{
		Block:      block,
		Round:      0,
		ProposerID: addr,
	}
	payload := proposal.SigningPayload()
	sig := ed25519.Sign(priv, payload)
	copy(proposal.Signature[:], sig)

	return proposal
}

func makeTestVote(t *testing.T) (*types.Vote, ed25519.PublicKey) {
	t.Helper()
	pub, priv := makeTestKeypair(t)
	addr := crypto.AddressFromPubKey(pub)

	vote := &types.Vote{
		BlockHash: crypto.HashSHA256([]byte("test-block")),
		Height:    1,
		Round:     0,
		VoterID:   addr,
	}
	payload := vote.SigningPayload()
	sig := ed25519.Sign(priv, payload)
	copy(vote.Signature[:], sig)

	return vote, pub
}

func makeTestTimeout(t *testing.T) *types.TimeoutMessage {
	t.Helper()
	pub, priv := makeTestKeypair(t)
	addr := crypto.AddressFromPubKey(pub)

	tm := &types.TimeoutMessage{
		Height:  1,
		Round:   0,
		VoterID: addr,
	}
	payload := tm.SigningPayload()
	sig := ed25519.Sign(priv, payload)
	copy(tm.Signature[:], sig)

	return tm
}

func makeTestHost(t *testing.T, port int) host.Host {
	t.Helper()
	priv, _, err := libp2pcrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generate libp2p key: %v", err)
	}
	addr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/127.0.0.1/tcp/%d", port))
	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrs(addr),
	)
	if err != nil {
		t.Fatalf("create host: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

// --- Protocol tests ---

func TestEncodeDecodeProposalRoundTrip(t *testing.T) {
	proposal := makeTestProposal(t)

	data, err := EncodeProposal(proposal)
	if err != nil {
		t.Fatalf("encode proposal: %v", err)
	}

	// First byte should be MsgProposal.
	if data[0] != byte(MsgProposal) {
		t.Fatalf("expected type byte 0x%02x, got 0x%02x", MsgProposal, data[0])
	}

	// Decode via generic decoder.
	msgType, decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode message: %v", err)
	}
	if msgType != MsgProposal {
		t.Fatalf("expected MsgProposal, got %v", msgType)
	}

	p := decoded.(*types.Proposal)
	if p.Round != proposal.Round {
		t.Fatalf("round mismatch: got %d, want %d", p.Round, proposal.Round)
	}
	if p.Block.Header.Height != proposal.Block.Header.Height {
		t.Fatalf("height mismatch: got %d, want %d", p.Block.Header.Height, proposal.Block.Header.Height)
	}
	if p.ProposerID != proposal.ProposerID {
		t.Fatal("proposer ID mismatch")
	}
	if p.Signature != proposal.Signature {
		t.Fatal("signature mismatch")
	}
}

func TestEncodeDecodeVoteRoundTrip(t *testing.T) {
	vote, _ := makeTestVote(t)

	data, err := EncodeVote(vote)
	if err != nil {
		t.Fatalf("encode vote: %v", err)
	}

	if data[0] != byte(MsgVote) {
		t.Fatalf("expected type byte 0x%02x, got 0x%02x", MsgVote, data[0])
	}

	msgType, decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode message: %v", err)
	}
	if msgType != MsgVote {
		t.Fatalf("expected MsgVote, got %v", msgType)
	}

	v := decoded.(*types.Vote)
	if v.BlockHash != vote.BlockHash {
		t.Fatal("block hash mismatch")
	}
	if v.Height != vote.Height {
		t.Fatalf("height mismatch: got %d, want %d", v.Height, vote.Height)
	}
	if v.VoterID != vote.VoterID {
		t.Fatal("voter ID mismatch")
	}
	if v.Signature != vote.Signature {
		t.Fatal("signature mismatch")
	}
}

func TestEncodeDecodeTimeoutRoundTrip(t *testing.T) {
	tm := makeTestTimeout(t)

	data, err := EncodeTimeout(tm)
	if err != nil {
		t.Fatalf("encode timeout: %v", err)
	}

	if data[0] != byte(MsgTimeout) {
		t.Fatalf("expected type byte 0x%02x, got 0x%02x", MsgTimeout, data[0])
	}

	msgType, decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("decode message: %v", err)
	}
	if msgType != MsgTimeout {
		t.Fatalf("expected MsgTimeout, got %v", msgType)
	}

	tm2 := decoded.(*types.TimeoutMessage)
	if tm2.Height != tm.Height {
		t.Fatalf("height mismatch: got %d, want %d", tm2.Height, tm.Height)
	}
	if tm2.Round != tm.Round {
		t.Fatalf("round mismatch: got %d, want %d", tm2.Round, tm.Round)
	}
	if tm2.VoterID != tm.VoterID {
		t.Fatal("voter ID mismatch")
	}
}

func TestDecodeRejectsUnknownType(t *testing.T) {
	data := []byte{0xFF, 0x01, 0x02, 0x03}
	_, _, err := DecodeMessage(data)
	if err == nil {
		t.Fatal("expected error for unknown message type")
	}
}

func TestDecodeRejectsEmpty(t *testing.T) {
	_, _, err := DecodeMessage(nil)
	if err == nil {
		t.Fatal("expected error for nil data")
	}
	_, _, err = DecodeMessage([]byte{})
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestDecodeRejectsOversize(t *testing.T) {
	data := make([]byte, MaxMessageSize+1)
	data[0] = byte(MsgVote)
	_, _, err := DecodeMessage(data)
	if err == nil {
		t.Fatal("expected error for oversize message")
	}
}

// --- Scoring tests ---

func TestScoringValidMessage(t *testing.T) {
	ps := NewPeerScoring()
	pid := peer.ID("test-peer")

	ps.RecordValidMessage(pid)
	ps.RecordValidMessage(pid)

	score := ps.Score(pid)
	if score != 2.0 {
		t.Fatalf("expected score 2.0, got %f", score)
	}
}

func TestScoringInvalidMessage(t *testing.T) {
	ps := NewPeerScoring()
	pid := peer.ID("test-peer")

	ps.RecordInvalidMessage(pid, "bad data")

	score := ps.Score(pid)
	if score != -10.0 {
		t.Fatalf("expected score -10.0, got %f", score)
	}
}

func TestScoringAutoBan(t *testing.T) {
	ps := NewPeerScoring()
	pid := peer.ID("test-peer")

	// 10 invalid messages = score -100 = auto-ban.
	for range 10 {
		ps.RecordInvalidMessage(pid, "spam")
	}

	if !ps.IsBanned(pid) {
		t.Fatal("expected peer to be auto-banned at -100 score")
	}
}

func TestScoringBanExpiry(t *testing.T) {
	ps := NewPeerScoring()
	pid := peer.ID("test-peer")

	// Ban for a tiny duration.
	ps.Ban(pid, "test", 1*time.Millisecond)
	if !ps.IsBanned(pid) {
		t.Fatal("expected peer to be banned")
	}

	time.Sleep(5 * time.Millisecond)
	if ps.IsBanned(pid) {
		t.Fatal("expected ban to have expired")
	}

	// CleanupExpiredBans should remove it.
	removed := ps.CleanupExpiredBans()
	if removed != 1 {
		t.Fatalf("expected 1 expired ban removed, got %d", removed)
	}
}

func TestScoringUnban(t *testing.T) {
	ps := NewPeerScoring()
	pid := peer.ID("test-peer")

	ps.Ban(pid, "test", 1*time.Hour)
	if !ps.IsBanned(pid) {
		t.Fatal("expected peer to be banned")
	}

	ps.Unban(pid)
	if ps.IsBanned(pid) {
		t.Fatal("expected peer to be unbanned")
	}

	// Score should be reset to 0.
	if score := ps.Score(pid); score != 0 {
		t.Fatalf("expected score 0 after unban, got %f", score)
	}
}

// --- Rate limiter tests ---

func TestRateLimiterAllows(t *testing.T) {
	rl := NewRateLimiter(DefaultRateLimitConfig())
	pid := peer.ID("test-peer")

	// First message should always be allowed (bucket starts full).
	if !rl.Allow(pid, MsgVote) {
		t.Fatal("expected first vote to be allowed")
	}
}

func TestRateLimiterBlocks(t *testing.T) {
	cfg := RateLimitConfig{
		ProposalRate:    1,
		VoteRate:        1,
		TimeoutRate:     1,
		GlobalRate:      2,
		BurstMultiplier: 1, // No burst — exactly 1 token.
	}
	rl := NewRateLimiter(cfg)
	pid := peer.ID("test-peer")

	// First message allowed.
	if !rl.Allow(pid, MsgVote) {
		t.Fatal("first vote should be allowed")
	}

	// Second immediate message should be blocked (type bucket exhausted).
	if rl.Allow(pid, MsgVote) {
		t.Fatal("second immediate vote should be blocked")
	}
}

func TestRateLimiterRefills(t *testing.T) {
	cfg := RateLimitConfig{
		ProposalRate:    100, // 100/s = refills fast
		VoteRate:        100,
		TimeoutRate:     100,
		GlobalRate:      200,
		BurstMultiplier: 1,
	}
	rl := NewRateLimiter(cfg)
	pid := peer.ID("test-peer")

	// Drain the bucket.
	rl.Allow(pid, MsgVote)

	// Wait a bit for refill.
	time.Sleep(20 * time.Millisecond)

	// Should be allowed again after refill.
	if !rl.Allow(pid, MsgVote) {
		t.Fatal("expected vote to be allowed after refill")
	}
}

func TestRateLimiterPerType(t *testing.T) {
	cfg := RateLimitConfig{
		ProposalRate:    1,
		VoteRate:        1,
		TimeoutRate:     1,
		GlobalRate:      100, // High global limit.
		BurstMultiplier: 1,
	}
	rl := NewRateLimiter(cfg)
	pid := peer.ID("test-peer")

	// Use up proposal bucket.
	rl.Allow(pid, MsgProposal)

	// Proposal blocked, but vote should still work (different type bucket).
	if rl.Allow(pid, MsgProposal) {
		t.Fatal("second proposal should be blocked")
	}
	if !rl.Allow(pid, MsgVote) {
		t.Fatal("vote should be allowed (separate bucket)")
	}
}

// --- Peer manager tests ---

func TestPeerManagerAddRemove(t *testing.T) {
	pm := NewPeerManager(10, NewPeerScoring())

	pid := peer.ID("test-peer-1")
	pm.AddPeer(&PeerInfo{ID: pid, Direction: Inbound})

	if pm.PeerCount() != 1 {
		t.Fatalf("expected 1 peer, got %d", pm.PeerCount())
	}

	peers := pm.ConnectedPeers()
	if len(peers) != 1 || peers[0] != pid {
		t.Fatal("ConnectedPeers mismatch")
	}

	pm.RemovePeer(pid)
	if pm.PeerCount() != 0 {
		t.Fatalf("expected 0 peers after remove, got %d", pm.PeerCount())
	}
}

func TestPeerManagerMaxPeers(t *testing.T) {
	pm := NewPeerManager(2, NewPeerScoring())

	pm.AddPeer(&PeerInfo{ID: peer.ID("p1"), Direction: Inbound})
	pm.AddPeer(&PeerInfo{ID: peer.ID("p2"), Direction: Inbound})

	// At max peers, should reject new connections.
	if pm.ShouldAcceptConnection(peer.ID("p3"), network.DirInbound) {
		t.Fatal("should reject when at max peers")
	}

	// Already connected peer should still be accepted.
	if !pm.ShouldAcceptConnection(peer.ID("p1"), network.DirInbound) {
		t.Fatal("already connected peer should be accepted")
	}
}

func TestPeerManagerValidatorPriority(t *testing.T) {
	scoring := NewPeerScoring()
	pm := NewPeerManager(2, scoring)

	pm.AddPeer(&PeerInfo{ID: peer.ID("p1"), Direction: Inbound})
	pm.AddPeer(&PeerInfo{ID: peer.ID("p2"), Direction: Inbound, IsValidator: true})

	// Give p1 a low score.
	scoring.RecordInvalidMessage(peer.ID("p1"), "bad")

	worst := pm.EvictWorstPeer()
	if worst != peer.ID("p1") {
		t.Fatalf("expected p1 to be evicted (non-validator, low score), got %s", worst)
	}
}

func TestPeerManagerBannedRejected(t *testing.T) {
	scoring := NewPeerScoring()
	pm := NewPeerManager(10, scoring)

	pid := peer.ID("bad-peer")
	scoring.Ban(pid, "malicious", 1*time.Hour)

	if pm.ShouldAcceptConnection(pid, network.DirInbound) {
		t.Fatal("banned peer should be rejected")
	}
}

// --- Discovery tests ---

func TestParseSeedAddrs(t *testing.T) {
	// Create a valid peer ID for testing.
	priv, _, _ := libp2pcrypto.GenerateEd25519Key(rand.Reader)
	pid, _ := peer.IDFromPrivateKey(priv)

	addrs := []string{
		fmt.Sprintf("/ip4/127.0.0.1/tcp/26656/p2p/%s", pid),
	}

	infos, err := ParseSeedAddrs(addrs)
	if err != nil {
		t.Fatalf("parse seed addrs: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 addr info, got %d", len(infos))
	}
	if infos[0].ID != pid {
		t.Fatal("peer ID mismatch")
	}
}

func TestParseSeedAddrsInvalid(t *testing.T) {
	// Invalid multiaddr.
	_, err := ParseSeedAddrs([]string{"not-a-multiaddr"})
	if err == nil {
		t.Fatal("expected error for invalid multiaddr")
	}

	// Valid multiaddr but missing /p2p/ component.
	_, err = ParseSeedAddrs([]string{"/ip4/127.0.0.1/tcp/26656"})
	if err == nil {
		t.Fatal("expected error for multiaddr without p2p component")
	}
}

// --- Integration tests ---

func TestTransportImplementsInterface(t *testing.T) {
	// This is a compile-time check via var _ consensus.Transport = (*P2PTransport)(nil)
	// in transport.go. This test simply verifies the type assertion at runtime.
	var transport interface{} = &P2PTransport{}
	if _, ok := transport.(interface {
		BroadcastProposal(*types.Proposal) error
		BroadcastVote(*types.Vote) error
		BroadcastTimeout(*types.TimeoutMessage) error
	}); !ok {
		t.Fatal("P2PTransport does not implement the Transport interface methods")
	}
}

func TestHostStartStop(t *testing.T) {
	pub, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	_ = pub

	ctx := context.Background()
	bh, err := NewHost(ctx, HostConfig{
		PrivateKey: priv,
		ListenAddr: "/ip4/127.0.0.1/tcp/0",
		MaxPeers:   10,
	})
	if err != nil {
		t.Fatalf("create host: %v", err)
	}

	if err := bh.Start(ctx); err != nil {
		t.Fatalf("start host: %v", err)
	}

	// Verify host has a peer ID and addresses.
	if bh.ID() == "" {
		t.Fatal("host should have a peer ID")
	}
	if len(bh.Addrs()) == 0 {
		t.Fatal("host should have listen addresses")
	}

	if err := bh.Stop(); err != nil {
		t.Fatalf("stop host: %v", err)
	}
}

func TestTwoNodeGossipRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create two hosts.
	_, priv1, _ := crypto.GenerateKeypair()
	_, priv2, _ := crypto.GenerateKeypair()

	host1, err := NewHost(ctx, HostConfig{
		PrivateKey: priv1,
		ListenAddr: "/ip4/127.0.0.1/tcp/0",
		MaxPeers:   10,
	})
	if err != nil {
		t.Fatalf("create host1: %v", err)
	}

	host2, err := NewHost(ctx, HostConfig{
		PrivateKey: priv2,
		ListenAddr: "/ip4/127.0.0.1/tcp/0",
		MaxPeers:   10,
	})
	if err != nil {
		t.Fatalf("create host2: %v", err)
	}

	// Start both hosts (joins consensus topic on each).
	if err := host1.Start(ctx); err != nil {
		t.Fatalf("start host1: %v", err)
	}
	if err := host2.Start(ctx); err != nil {
		t.Fatalf("start host2: %v", err)
	}
	defer host1.Stop()
	defer host2.Stop()

	// Create transports and subscribe BEFORE connecting, so GossipSub
	// has active subscriptions when the mesh forms.
	transport1 := NewP2PTransport(host1, nil, nil)
	transport2 := NewP2PTransport(host2, nil, nil)

	// transport1 also needs a subscription for GossipSub mesh to form.
	if err := transport1.Start(ctx); err != nil {
		t.Fatalf("start transport1: %v", err)
	}
	defer transport1.Stop()

	sub2 := transport2.Subscribe()
	if err := transport2.Start(ctx); err != nil {
		t.Fatalf("start transport2: %v", err)
	}
	defer transport2.Stop()

	// Connect host2 to host1.
	host1Info := peer.AddrInfo{
		ID:    host1.ID(),
		Addrs: host1.LibP2PHost().Addrs(),
	}
	if err := host2.LibP2PHost().Connect(ctx, host1Info); err != nil {
		t.Fatalf("connect host2 to host1: %v", err)
	}

	// Wait for GossipSub mesh to form (needs heartbeat cycles).
	time.Sleep(3 * time.Second)

	// --- Test proposal round-trip ---
	proposal := makeTestProposal(t)
	if err := transport1.BroadcastProposal(proposal); err != nil {
		t.Fatalf("broadcast proposal: %v", err)
	}

	select {
	case received := <-sub2.Proposals:
		if received.Round != proposal.Round {
			t.Fatalf("proposal round mismatch: got %d, want %d", received.Round, proposal.Round)
		}
		if received.Block.Header.Height != proposal.Block.Header.Height {
			t.Fatalf("proposal height mismatch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for proposal")
	}

	// --- Test vote round-trip ---
	vote, _ := makeTestVote(t)
	if err := transport1.BroadcastVote(vote); err != nil {
		t.Fatalf("broadcast vote: %v", err)
	}

	select {
	case received := <-sub2.Votes:
		if received.BlockHash != vote.BlockHash {
			t.Fatal("vote block hash mismatch")
		}
		if received.VoterID != vote.VoterID {
			t.Fatal("vote voter ID mismatch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for vote")
	}

	// --- Test timeout round-trip ---
	tm := makeTestTimeout(t)
	if err := transport1.BroadcastTimeout(tm); err != nil {
		t.Fatalf("broadcast timeout: %v", err)
	}

	select {
	case received := <-sub2.Timeouts:
		if received.Height != tm.Height {
			t.Fatalf("timeout height mismatch: got %d, want %d", received.Height, tm.Height)
		}
		if received.VoterID != tm.VoterID {
			t.Fatal("timeout voter ID mismatch")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for timeout message")
	}
}

func TestMessageValidationRejectsInvalid(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, priv1, _ := crypto.GenerateKeypair()
	_, priv2, _ := crypto.GenerateKeypair()

	host1, err := NewHost(ctx, HostConfig{
		PrivateKey: priv1,
		ListenAddr: "/ip4/127.0.0.1/tcp/0",
		MaxPeers:   10,
	})
	if err != nil {
		t.Fatalf("create host1: %v", err)
	}

	host2, err := NewHost(ctx, HostConfig{
		PrivateKey: priv2,
		ListenAddr: "/ip4/127.0.0.1/tcp/0",
		MaxPeers:   10,
	})
	if err != nil {
		t.Fatalf("create host2: %v", err)
	}

	if err := host1.Start(ctx); err != nil {
		t.Fatalf("start host1: %v", err)
	}
	if err := host2.Start(ctx); err != nil {
		t.Fatalf("start host2: %v", err)
	}
	defer host1.Stop()
	defer host2.Stop()

	// Create a validator set with a specific validator.
	vPub, _ := makeTestKeypair(t)
	vAddr := crypto.AddressFromPubKey(vPub)
	valSet, _ := types.NewValidatorSet([]types.Validator{
		{Address: vAddr, PublicKey: crypto.PubKeyTo32(vPub), VotingPower: 100},
	})

	// Set up transports and subscriptions BEFORE connecting.
	transport1 := NewP2PTransport(host1, nil, nil)
	if err := transport1.Start(ctx); err != nil {
		t.Fatalf("start transport1: %v", err)
	}
	defer transport1.Stop()

	transport2 := NewP2PTransport(host2, valSet, nil)
	sub2 := transport2.Subscribe()
	if err := transport2.Start(ctx); err != nil {
		t.Fatalf("start transport2: %v", err)
	}
	defer transport2.Stop()

	// Connect hosts.
	host1Info := peer.AddrInfo{
		ID:    host1.ID(),
		Addrs: host1.LibP2PHost().Addrs(),
	}
	if err := host2.LibP2PHost().Connect(ctx, host1Info); err != nil {
		t.Fatalf("connect: %v", err)
	}
	time.Sleep(3 * time.Second)

	// Send a vote from an unknown validator (not in valSet).
	unknownVote, _ := makeTestVote(t) // creates vote with random keypair
	if err := transport1.BroadcastVote(unknownVote); err != nil {
		t.Fatalf("broadcast invalid vote: %v", err)
	}

	// The vote should NOT be delivered (unknown validator).
	select {
	case <-sub2.Votes:
		t.Fatal("expected invalid vote to be rejected, but it was delivered")
	case <-time.After(3 * time.Second):
		// Good — rejected as expected.
	}
}

// --- MessageType String tests ---

func TestMessageTypeString(t *testing.T) {
	tests := []struct {
		mt   MessageType
		want string
	}{
		{MsgProposal, "proposal"},
		{MsgVote, "vote"},
		{MsgTimeout, "timeout"},
		{MessageType(0xFF), "unknown(0xff)"},
	}
	for _, tt := range tests {
		if got := tt.mt.String(); got != tt.want {
			t.Errorf("MessageType(%d).String() = %q, want %q", tt.mt, got, tt.want)
		}
	}
}

// --- Envelope tests ---

func TestEnvelopeEncodeDecode(t *testing.T) {
	payload := []byte{0x01, 0x02, 0x03}
	env := &Envelope{Type: MsgVote, Payload: payload}

	data := env.Encode()
	if len(data) != 4 {
		t.Fatalf("encoded length = %d, want 4", len(data))
	}
	if data[0] != byte(MsgVote) {
		t.Fatalf("type byte = 0x%02x, want 0x%02x", data[0], MsgVote)
	}

	decoded, err := DecodeEnvelope(data)
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if decoded.Type != MsgVote {
		t.Fatalf("decoded type = %v, want %v", decoded.Type, MsgVote)
	}
	if len(decoded.Payload) != 3 {
		t.Fatalf("decoded payload length = %d, want 3", len(decoded.Payload))
	}
}

// --- PeerManager additional tests ---

func TestPeerManagerMarkValidator(t *testing.T) {
	pm := NewPeerManager(10, NewPeerScoring())
	pid := peer.ID("validator-1")
	pm.AddPeer(&PeerInfo{ID: pid, Direction: Outbound})

	var addr types.Address
	copy(addr[:], []byte("validator-address-padded-to-32!"))
	pm.MarkValidator(pid, addr)

	info, ok := pm.GetPeer(pid)
	if !ok {
		t.Fatal("peer not found")
	}
	if !info.IsValidator {
		t.Fatal("expected peer to be marked as validator")
	}
	if info.ValidatorAddr != addr {
		t.Fatal("validator address mismatch")
	}
}

func TestPeerManagerOutboundCount(t *testing.T) {
	pm := NewPeerManager(10, NewPeerScoring())
	pm.AddPeer(&PeerInfo{ID: peer.ID("in1"), Direction: Inbound})
	pm.AddPeer(&PeerInfo{ID: peer.ID("out1"), Direction: Outbound})
	pm.AddPeer(&PeerInfo{ID: peer.ID("out2"), Direction: Outbound})

	if pm.OutboundCount() != 2 {
		t.Fatalf("expected 2 outbound, got %d", pm.OutboundCount())
	}
}

// --- Scoring additional tests ---

func TestScoringBannedCount(t *testing.T) {
	ps := NewPeerScoring()
	ps.Ban(peer.ID("p1"), "test", 1*time.Hour)
	ps.Ban(peer.ID("p2"), "test", 1*time.Hour)

	if ps.BannedCount() != 2 {
		t.Fatalf("expected 2 banned, got %d", ps.BannedCount())
	}
}

// --- RateLimiter cleanup test ---

func TestRateLimiterCleanup(t *testing.T) {
	rl := NewRateLimiter(DefaultRateLimitConfig())
	pid := peer.ID("old-peer")
	rl.Allow(pid, MsgVote)

	// Cleanup with zero stale duration — should remove the peer.
	removed := rl.Cleanup(0)
	if removed != 1 {
		t.Fatalf("expected 1 stale peer removed, got %d", removed)
	}
}
