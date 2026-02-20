package consensus

import (
	"testing"
	"time"

	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// --- Test helpers ---

type testValidator struct {
	pub     crypto.PublicKey
	priv    crypto.PrivateKey
	address types.Address
	pubKey  [32]byte
}

func newTestValidator(t *testing.T) testValidator {
	t.Helper()
	pub, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	addr := crypto.AddressFromPubKey(pub)
	return testValidator{
		pub:     pub,
		priv:    priv,
		address: addr,
		pubKey:  crypto.PubKeyTo32(pub),
	}
}

func makeValidatorSet(t *testing.T, vals []testValidator) *types.ValidatorSet {
	t.Helper()
	vs := make([]types.Validator, len(vals))
	for i, v := range vals {
		vs[i] = types.Validator{
			Address:     v.address,
			PublicKey:   v.pubKey,
			VotingPower: 100,
		}
	}
	valSet, err := types.NewValidatorSet(vs)
	if err != nil {
		t.Fatalf("new validator set: %v", err)
	}
	return valSet
}

func signVote(t *testing.T, v testValidator, blockHash types.Hash, height, round uint64) *types.Vote {
	t.Helper()
	vote := &types.Vote{
		BlockHash: blockHash,
		Height:    height,
		Round:     round,
		VoterID:   v.address,
	}
	payload := vote.SigningPayload()
	sig := crypto.Sign(v.priv, payload)
	vote.Signature = crypto.SigTo64(sig)
	return vote
}

// mockTransport records broadcast calls.
type mockTransport struct {
	proposals []*types.Proposal
	votes     []*types.Vote
	timeouts  []*types.TimeoutMessage
}

func (m *mockTransport) BroadcastProposal(p *types.Proposal) error {
	m.proposals = append(m.proposals, p)
	return nil
}
func (m *mockTransport) BroadcastVote(v *types.Vote) error {
	m.votes = append(m.votes, v)
	return nil
}
func (m *mockTransport) BroadcastTimeout(msg *types.TimeoutMessage) error {
	m.timeouts = append(m.timeouts, msg)
	return nil
}

// mockExecutor returns a fixed state root.
type mockExecutor struct {
	stateRoot types.Hash
}

func (m *mockExecutor) ExecuteBlock(_ *types.Block, _ types.Hash) (*ExecutionResult, error) {
	return &ExecutionResult{StateRoot: m.stateRoot}, nil
}

// mockTxProvider returns canned transactions.
type mockTxProvider struct {
	txs [][]byte
}

func (m *mockTxProvider) ReapMaxTxs(_ int) [][]byte {
	return m.txs
}

// --- VoteSet tests ---

func TestVoteSetAddAndQuorum(t *testing.T) {
	vals := make([]testValidator, 4)
	for i := range vals {
		vals[i] = newTestValidator(t)
	}
	valSet := makeValidatorSet(t, vals)

	vs := NewVoteSet(1, 0, valSet)
	blockHash := crypto.HashSHA256([]byte("block1"))

	// Add 2 votes — should not reach quorum (need 2f+1 = 199, have 200).
	for i := range 2 {
		vote := signVote(t, vals[i], blockHash, 1, 0)
		quorum, _, err := vs.AddVote(vote)
		if err != nil {
			t.Fatalf("add vote %d: %v", i, err)
		}
		if quorum {
			t.Fatalf("quorum reached with only %d votes", i+1)
		}
	}

	// Add 3rd vote — should reach quorum (300 power, quorum=199, have 300).
	vote := signVote(t, vals[2], blockHash, 1, 0)
	quorum, _, err := vs.AddVote(vote)
	if err != nil {
		t.Fatalf("add vote 3: %v", err)
	}
	if !quorum {
		t.Fatal("expected quorum with 3 votes (300 power)")
	}
}

func TestVoteSetRejectsWrongHeight(t *testing.T) {
	v := newTestValidator(t)
	valSet := makeValidatorSet(t, []testValidator{v})
	vs := NewVoteSet(1, 0, valSet)

	blockHash := crypto.HashSHA256([]byte("block"))
	vote := signVote(t, v, blockHash, 2, 0) // wrong height
	_, _, err := vs.AddVote(vote)
	if err == nil {
		t.Fatal("expected error for wrong height")
	}
}

func TestVoteSetRejectsUnknownValidator(t *testing.T) {
	v1 := newTestValidator(t)
	v2 := newTestValidator(t)
	valSet := makeValidatorSet(t, []testValidator{v1})
	vs := NewVoteSet(1, 0, valSet)

	blockHash := crypto.HashSHA256([]byte("block"))
	vote := signVote(t, v2, blockHash, 1, 0) // unknown validator
	_, _, err := vs.AddVote(vote)
	if err == nil {
		t.Fatal("expected error for unknown validator")
	}
}

func TestVoteSetEquivocationDetection(t *testing.T) {
	v := newTestValidator(t)
	valSet := makeValidatorSet(t, []testValidator{v})
	vs := NewVoteSet(1, 0, valSet)

	hash1 := crypto.HashSHA256([]byte("block1"))
	hash2 := crypto.HashSHA256([]byte("block2"))

	vote1 := signVote(t, v, hash1, 1, 0)
	_, _, err := vs.AddVote(vote1)
	if err != nil {
		t.Fatalf("add vote1: %v", err)
	}

	vote2 := signVote(t, v, hash2, 1, 0)
	_, evidence, err := vs.AddVote(vote2)
	if err == nil {
		t.Fatal("expected error for equivocation")
	}
	if evidence == nil {
		t.Fatal("expected slashing evidence")
	}
	if evidence.DoubleVote == nil {
		t.Fatal("expected double vote evidence")
	}
}

func TestVoteSetDuplicateVoteSameBlock(t *testing.T) {
	v := newTestValidator(t)
	valSet := makeValidatorSet(t, []testValidator{v})
	vs := NewVoteSet(1, 0, valSet)

	blockHash := crypto.HashSHA256([]byte("block"))
	vote := signVote(t, v, blockHash, 1, 0)

	_, _, err := vs.AddVote(vote)
	if err != nil {
		t.Fatalf("add vote: %v", err)
	}

	// Same vote again — should be a no-op.
	_, evidence, err := vs.AddVote(vote)
	if err != nil {
		t.Fatalf("duplicate vote should not error: %v", err)
	}
	if evidence != nil {
		t.Fatal("duplicate vote should not produce evidence")
	}
}

func TestVoteSetRejectsInvalidSignature(t *testing.T) {
	v := newTestValidator(t)
	valSet := makeValidatorSet(t, []testValidator{v})
	vs := NewVoteSet(1, 0, valSet)

	vote := &types.Vote{
		BlockHash: crypto.HashSHA256([]byte("block")),
		Height:    1,
		Round:     0,
		VoterID:   v.address,
		Signature: [64]byte{1, 2, 3}, // invalid sig
	}
	_, _, err := vs.AddVote(vote)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

// --- QC tests ---

func TestMakeQCRequiresQuorum(t *testing.T) {
	v := newTestValidator(t)
	valSet := makeValidatorSet(t, []testValidator{v})
	vs := NewVoteSet(1, 0, valSet)

	// No votes yet.
	_, err := vs.MakeQC()
	if err == nil {
		t.Fatal("expected error: no quorum")
	}
}

func TestMakeQCSuccess(t *testing.T) {
	vals := make([]testValidator, 3)
	for i := range vals {
		vals[i] = newTestValidator(t)
	}
	valSet := makeValidatorSet(t, vals)
	vs := NewVoteSet(1, 0, valSet)

	blockHash := crypto.HashSHA256([]byte("block"))
	for _, v := range vals {
		vote := signVote(t, v, blockHash, 1, 0)
		vs.AddVote(vote)
	}

	qc, err := vs.MakeQC()
	if err != nil {
		t.Fatalf("make QC: %v", err)
	}
	if qc.BlockHash != blockHash {
		t.Fatal("QC block hash mismatch")
	}
	if qc.Round != 0 {
		t.Fatalf("QC round = %d, want 0", qc.Round)
	}
	if len(qc.Votes) != 3 {
		t.Fatalf("QC votes = %d, want 3", len(qc.Votes))
	}
}

func TestForkChoice(t *testing.T) {
	a := &types.QuorumCertificate{Round: 5, BlockHash: crypto.HashSHA256([]byte("a"))}
	b := &types.QuorumCertificate{Round: 3, BlockHash: crypto.HashSHA256([]byte("b"))}

	// Higher round wins.
	if got := ForkChoice(a, b); got != a {
		t.Fatal("expected a (higher round)")
	}
	if got := ForkChoice(b, a); got != a {
		t.Fatal("expected a (higher round)")
	}

	// Nil handling.
	if got := ForkChoice(nil, b); got != b {
		t.Fatal("expected b when a is nil")
	}
	if got := ForkChoice(a, nil); got != a {
		t.Fatal("expected a when b is nil")
	}
}

// --- Timeout tests ---

func TestTimeoutSchedulerExponentialBackoff(t *testing.T) {
	ts := NewTimeoutScheduler(1000, 60000)

	d0 := ts.TimeoutDuration(0)
	d1 := ts.TimeoutDuration(1)
	d2 := ts.TimeoutDuration(2)

	if d0 != 1*time.Second {
		t.Fatalf("round 0 timeout = %v, want 1s", d0)
	}
	if d1 != 2*time.Second {
		t.Fatalf("round 1 timeout = %v, want 2s", d1)
	}
	if d2 != 4*time.Second {
		t.Fatalf("round 2 timeout = %v, want 4s", d2)
	}
}

func TestTimeoutSchedulerCap(t *testing.T) {
	ts := NewTimeoutScheduler(1000, 5000)
	d := ts.TimeoutDuration(10) // 1000 * 2^10 = 1024000ms > 5000ms cap
	if d != 5*time.Second {
		t.Fatalf("capped timeout = %v, want 5s", d)
	}
}

func TestTimeoutSchedulerResetAfterCommit(t *testing.T) {
	ts := NewTimeoutScheduler(1000, 60000)

	// After some rounds.
	d5 := ts.TimeoutDuration(5) // 2^5 * 1s = 32s

	// Reset at round 5.
	ts.Reset(5)

	// Round 6 should be base * 2^1 = 2s (round 6 - lastCommit 5 = 1).
	d6 := ts.TimeoutDuration(6)

	if d5 != 32*time.Second {
		t.Fatalf("round 5 before reset = %v, want 32s", d5)
	}
	if d6 != 2*time.Second {
		t.Fatalf("round 6 after reset = %v, want 2s", d6)
	}
}

func TestTimeoutScheduleChannel(t *testing.T) {
	ts := NewTimeoutScheduler(50, 60000) // 50ms base
	ch := ts.ScheduleTimeout(0)

	select {
	case <-ch:
		// OK — timer fired.
	case <-time.After(2 * time.Second):
		t.Fatal("timeout did not fire")
	}
}

// --- EvidencePool tests ---

func TestEvidencePoolAddAndRetrieve(t *testing.T) {
	ep := NewEvidencePool()

	v := newTestValidator(t)
	ev := &types.SlashingEvidence{
		DoubleVote: &types.DoubleVoteEvidence{
			ValidatorID: v.address,
		},
		Height: 1,
	}

	if err := ep.AddEvidence(ev); err != nil {
		t.Fatalf("add evidence: %v", err)
	}
	if ep.Size() != 1 {
		t.Fatalf("evidence pool size = %d, want 1", ep.Size())
	}
	if !ep.HasEvidence(v.address) {
		t.Fatal("expected evidence for validator")
	}

	pending := ep.GetPendingEvidence()
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
}

func TestEvidencePoolDeduplicate(t *testing.T) {
	ep := NewEvidencePool()
	v := newTestValidator(t)

	ev1 := &types.SlashingEvidence{
		DoubleVote: &types.DoubleVoteEvidence{ValidatorID: v.address},
		Height:     1,
	}
	ev2 := &types.SlashingEvidence{
		DoubleVote: &types.DoubleVoteEvidence{ValidatorID: v.address},
		Height:     2,
	}

	ep.AddEvidence(ev1)
	ep.AddEvidence(ev2) // should be a no-op

	if ep.Size() != 1 {
		t.Fatalf("evidence pool size = %d, want 1 (deduplicated)", ep.Size())
	}
}

func TestEvidencePoolRejectsNil(t *testing.T) {
	ep := NewEvidencePool()
	if err := ep.AddEvidence(nil); err == nil {
		t.Fatal("expected error for nil evidence")
	}
}

// --- CommitRule tests ---

func TestCheckCommitRuleNilQC(t *testing.T) {
	state := &ConsensusState{}
	ok, _ := CheckCommitRule(nil, nil, state)
	if ok {
		t.Fatal("should not commit with nil QC")
	}
}

func TestCheckCommitRuleTwoChain(t *testing.T) {
	lockedBlock := &types.Block{
		Header: types.BlockHeader{Height: 1},
	}
	highQC := &types.QuorumCertificate{Round: 1}
	blockQC := &types.QuorumCertificate{Round: 2}

	state := &ConsensusState{
		LockedBlock: lockedBlock,
		HighestQC:   highQC,
	}

	ok, commitBlock := CheckCommitRule(
		&types.Block{Header: types.BlockHeader{Height: 2}},
		blockQC,
		state,
	)
	if !ok {
		t.Fatal("expected commit")
	}
	if commitBlock != lockedBlock {
		t.Fatal("expected locked block to be committed")
	}
}

// --- ConsensusState tests ---

func TestConsensusStateLocking(t *testing.T) {
	v := newTestValidator(t)
	valSet := makeValidatorSet(t, []testValidator{v})
	cs := NewConsensusState(1, valSet)

	if cs.IsLocked() {
		t.Fatal("should not be locked initially")
	}

	block := &types.Block{Header: types.BlockHeader{Height: 1}}
	cs.Lock(block, 0)
	if !cs.IsLocked() {
		t.Fatal("should be locked")
	}
	if cs.LockedRound != 0 {
		t.Fatalf("locked round = %d, want 0", cs.LockedRound)
	}

	cs.Unlock()
	if cs.IsLocked() {
		t.Fatal("should be unlocked")
	}
}

func TestConsensusStateResetForNewRound(t *testing.T) {
	v := newTestValidator(t)
	valSet := makeValidatorSet(t, []testValidator{v})
	cs := NewConsensusState(1, valSet)

	cs.Proposal = &types.Proposal{}
	cs.ResetForNewRound(1, valSet)

	if cs.Round != 1 {
		t.Fatalf("round = %d, want 1", cs.Round)
	}
	if cs.Step != StepPropose {
		t.Fatal("step should be Propose")
	}
	if cs.Proposal != nil {
		t.Fatal("proposal should be nil after reset")
	}
}

func TestConsensusStateResetForNewHeight(t *testing.T) {
	v := newTestValidator(t)
	valSet := makeValidatorSet(t, []testValidator{v})
	cs := NewConsensusState(1, valSet)

	cs.Lock(&types.Block{}, 0)
	cs.ResetForNewHeight(2, valSet)

	if cs.Height != 2 {
		t.Fatalf("height = %d, want 2", cs.Height)
	}
	if cs.Round != 0 {
		t.Fatalf("round = %d, want 0", cs.Round)
	}
}

func TestConsensusStateUpdateHighestQC(t *testing.T) {
	v := newTestValidator(t)
	valSet := makeValidatorSet(t, []testValidator{v})
	cs := NewConsensusState(1, valSet)

	qc1 := &types.QuorumCertificate{Round: 1}
	qc2 := &types.QuorumCertificate{Round: 3}
	qc3 := &types.QuorumCertificate{Round: 2}

	cs.UpdateHighestQC(qc1)
	if cs.HighestQC.Round != 1 {
		t.Fatal("highest QC should be round 1")
	}

	cs.UpdateHighestQC(qc2)
	if cs.HighestQC.Round != 3 {
		t.Fatal("highest QC should be round 3")
	}

	cs.UpdateHighestQC(qc3) // lower — should not update
	if cs.HighestQC.Round != 3 {
		t.Fatal("highest QC should still be round 3")
	}
}

// --- ProposerRotation tests ---

func TestProposerRotation(t *testing.T) {
	vals := make([]testValidator, 4)
	for i := range vals {
		vals[i] = newTestValidator(t)
	}
	valSet := makeValidatorSet(t, vals)

	// (height + round) % 4 determines proposer.
	for h := range uint64(8) {
		for r := range uint64(4) {
			expected := (h + r) % 4
			proposer := valSet.GetProposer(h, r)
			if proposer.Address != vals[expected].address {
				t.Errorf("h=%d r=%d: got proposer %s, want %s",
					h, r, proposer.Address, vals[expected].address)
			}
		}
	}
}
