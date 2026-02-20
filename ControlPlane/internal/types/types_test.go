package types_test

import (
	"testing"

	"github.com/echenim/Bedrock/controlplane/internal/crypto"
	"github.com/echenim/Bedrock/controlplane/internal/types"
)

// --- Hash & Address ---

func TestHashFromBytesRoundTrip(t *testing.T) {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i)
	}
	h, err := types.HashFromBytes(b)
	if err != nil {
		t.Fatalf("HashFromBytes: %v", err)
	}
	if h.IsZero() {
		t.Fatal("hash should not be zero")
	}
	if h.String() != "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f" {
		t.Fatalf("unexpected hex: %s", h.String())
	}
}

func TestHashFromBytesRejectsWrongLength(t *testing.T) {
	_, err := types.HashFromBytes([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("should reject wrong length")
	}
}

func TestHashFromHex(t *testing.T) {
	hexStr := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	h, err := types.HashFromHex(hexStr)
	if err != nil {
		t.Fatalf("HashFromHex: %v", err)
	}
	if h.String() != hexStr {
		t.Fatalf("round-trip mismatch: got %s", h.String())
	}
}

func TestAddressFromBytesRoundTrip(t *testing.T) {
	b := make([]byte, 32)
	b[0] = 0xff
	a, err := types.AddressFromBytes(b)
	if err != nil {
		t.Fatalf("AddressFromBytes: %v", err)
	}
	if a.IsZero() {
		t.Fatal("address should not be zero")
	}
}

func TestZeroHash(t *testing.T) {
	var h types.Hash
	if !h.IsZero() {
		t.Fatal("default hash should be zero")
	}
	if h != types.ZeroHash {
		t.Fatal("default hash should equal ZeroHash")
	}
}

// --- Block ---

func TestBlockHeaderComputeHashDeterministic(t *testing.T) {
	header := types.BlockHeader{
		Height:    1,
		Round:     0,
		BlockTime: 1700000000,
		ChainID:   []byte("test-chain"),
	}

	h1 := header.ComputeHash()
	h2 := header.ComputeHash()
	if h1 != h2 {
		t.Fatal("ComputeHash should be deterministic")
	}
	if h1.IsZero() {
		t.Fatal("hash should not be zero")
	}
}

func TestBlockHeaderComputeHashDiffersOnChange(t *testing.T) {
	hdr1 := &types.BlockHeader{Height: 1, ChainID: []byte("c")}
	hdr2 := &types.BlockHeader{Height: 2, ChainID: []byte("c")}
	h1 := hdr1.ComputeHash()
	h2 := hdr2.ComputeHash()
	if h1 == h2 {
		t.Fatal("different heights should produce different hashes")
	}
}

func TestBlockValidateGenesis(t *testing.T) {
	b := &types.Block{
		Header: types.BlockHeader{},
	}
	if err := b.Validate(); err != nil {
		t.Fatalf("genesis block should be valid: %v", err)
	}
}

func TestBlockValidateRejectsZeroHeight(t *testing.T) {
	b := &types.Block{
		Header: types.BlockHeader{
			Round:      1,
			ParentHash: crypto.HashSHA256([]byte("parent")),
		},
	}
	if err := b.Validate(); err == nil {
		t.Fatal("should reject non-genesis block with height=0")
	}
}

func TestBlockValidateRejectsEmptyChainID(t *testing.T) {
	b := &types.Block{
		Header: types.BlockHeader{
			Height:     1,
			ProposerID: makeTestAddress(1),
		},
	}
	if err := b.Validate(); err == nil {
		t.Fatal("should reject empty chain_id")
	}
}

func TestBlockToProtoRoundTrip(t *testing.T) {
	b := &types.Block{
		Header: types.BlockHeader{
			Height:     10,
			Round:      1,
			ParentHash: crypto.HashSHA256([]byte("parent")),
			StateRoot:  crypto.HashSHA256([]byte("state")),
			TxRoot:     crypto.HashSHA256([]byte("txroot")),
			ProposerID: makeTestAddress(1),
			BlockTime:  1700000000,
			ChainID:    []byte("test-chain"),
		},
		Transactions: [][]byte{[]byte("tx1"), []byte("tx2")},
	}
	b.Header.BlockHash = b.Header.ComputeHash()

	pb := b.ToProto()
	decoded, err := types.BlockFromProto(pb)
	if err != nil {
		t.Fatalf("BlockFromProto: %v", err)
	}

	if decoded.Header.Height != b.Header.Height {
		t.Fatal("height mismatch")
	}
	if decoded.Header.BlockHash != b.Header.BlockHash {
		t.Fatal("block hash mismatch")
	}
	if len(decoded.Transactions) != 2 {
		t.Fatalf("expected 2 txs, got %d", len(decoded.Transactions))
	}
}

// --- Vote ---

func TestVoteSignAndVerify(t *testing.T) {
	pub, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}
	addr := crypto.AddressFromPubKey(pub)

	vote := &types.Vote{
		BlockHash: crypto.HashSHA256([]byte("block")),
		Height:    10,
		Round:     1,
		VoterID:   addr,
	}

	sig := crypto.Sign(priv, vote.SigningPayload())
	vote.Signature = crypto.SigTo64(sig)

	pk32 := crypto.PubKeyTo32(pub)
	if !vote.Verify(pk32) {
		t.Fatal("valid vote should verify")
	}
}

func TestVoteVerifyRejectsInvalid(t *testing.T) {
	pub, priv, err := crypto.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}
	addr := crypto.AddressFromPubKey(pub)

	vote := &types.Vote{
		BlockHash: crypto.HashSHA256([]byte("block")),
		Height:    10,
		Round:     1,
		VoterID:   addr,
	}

	sig := crypto.Sign(priv, vote.SigningPayload())
	vote.Signature = crypto.SigTo64(sig)

	// Wrong key.
	pub2, _, _ := crypto.GenerateKeypair()
	pk2 := crypto.PubKeyTo32(pub2)
	if vote.Verify(pk2) {
		t.Fatal("should reject wrong public key")
	}

	// Zero signature.
	vote2 := *vote
	vote2.Signature = [64]byte{}
	if vote2.Verify(crypto.PubKeyTo32(pub)) {
		t.Fatal("should reject zero signature")
	}
}

func TestVoteToProtoRoundTrip(t *testing.T) {
	pub, priv, _ := crypto.GenerateKeypair()
	addr := crypto.AddressFromPubKey(pub)

	vote := &types.Vote{
		BlockHash: crypto.HashSHA256([]byte("block")),
		Height:    5,
		Round:     2,
		VoterID:   addr,
	}
	vote.Signature = crypto.SigTo64(crypto.Sign(priv, vote.SigningPayload()))

	pb := vote.ToProto()
	decoded, err := types.VoteFromProto(pb)
	if err != nil {
		t.Fatalf("VoteFromProto: %v", err)
	}
	if decoded.BlockHash != vote.BlockHash || decoded.Height != vote.Height {
		t.Fatal("vote round-trip mismatch")
	}
}

func TestIsEquivocation(t *testing.T) {
	voter := makeTestAddress(1)

	a := &types.Vote{
		BlockHash: crypto.HashSHA256([]byte("block-A")),
		Height:    10,
		Round:     1,
		VoterID:   voter,
	}
	b := &types.Vote{
		BlockHash: crypto.HashSHA256([]byte("block-B")),
		Height:    10,
		Round:     1,
		VoterID:   voter,
	}

	if !types.IsEquivocation(a, b) {
		t.Fatal("should detect equivocation: same voter, same (h,r), different block")
	}

	// Same block -> not equivocation.
	c := &types.Vote{
		BlockHash: a.BlockHash,
		Height:    10,
		Round:     1,
		VoterID:   voter,
	}
	if types.IsEquivocation(a, c) {
		t.Fatal("same block should not be equivocation")
	}

	// Different voter -> not equivocation.
	d := &types.Vote{
		BlockHash: crypto.HashSHA256([]byte("block-B")),
		Height:    10,
		Round:     1,
		VoterID:   makeTestAddress(2),
	}
	if types.IsEquivocation(a, d) {
		t.Fatal("different voter should not be equivocation")
	}

	// Different round -> not equivocation.
	e := &types.Vote{
		BlockHash: crypto.HashSHA256([]byte("block-B")),
		Height:    10,
		Round:     2,
		VoterID:   voter,
	}
	if types.IsEquivocation(a, e) {
		t.Fatal("different round should not be equivocation")
	}
}

// --- QuorumCertificate ---

func TestQCVerifyAcceptsQuorum(t *testing.T) {
	valSet, keys := makeTestValidatorSet(4, 100)
	blockHash := crypto.HashSHA256([]byte("block"))

	votes := make([]types.Vote, 3) // 3 out of 4 = 300/400 power, quorum = 2*(400-1)/3 + 1 = 267
	for i := range 3 {
		votes[i] = makeSignedVote(blockHash, 10, 1, valSet.Validators[i].Address, keys[i])
	}

	qc := &types.QuorumCertificate{
		BlockHash: blockHash,
		Round:     1,
		Votes:     votes,
	}

	if err := qc.Verify(valSet); err != nil {
		t.Fatalf("QC with quorum should verify: %v", err)
	}
}

func TestQCVerifyRejectsBelowQuorum(t *testing.T) {
	valSet, keys := makeTestValidatorSet(4, 100)
	blockHash := crypto.HashSHA256([]byte("block"))

	// Only 1 vote = 100 power, quorum = 267 => should fail.
	votes := []types.Vote{
		makeSignedVote(blockHash, 10, 1, valSet.Validators[0].Address, keys[0]),
	}

	qc := &types.QuorumCertificate{
		BlockHash: blockHash,
		Round:     1,
		Votes:     votes,
	}

	if err := qc.Verify(valSet); err == nil {
		t.Fatal("QC below quorum should fail verification")
	}
}

func TestQCVerifyRejectsInvalidSignature(t *testing.T) {
	valSet, keys := makeTestValidatorSet(4, 100)
	blockHash := crypto.HashSHA256([]byte("block"))

	votes := make([]types.Vote, 3)
	for i := range 3 {
		votes[i] = makeSignedVote(blockHash, 10, 1, valSet.Validators[i].Address, keys[i])
	}
	// Corrupt one signature.
	votes[1].Signature[0] ^= 0xff

	qc := &types.QuorumCertificate{
		BlockHash: blockHash,
		Round:     1,
		Votes:     votes,
	}

	if err := qc.Verify(valSet); err == nil {
		t.Fatal("QC with invalid signature should fail verification")
	}
}

func TestQCVotingPower(t *testing.T) {
	valSet, keys := makeTestValidatorSet(4, 100)
	blockHash := crypto.HashSHA256([]byte("block"))

	votes := make([]types.Vote, 2)
	for i := range 2 {
		votes[i] = makeSignedVote(blockHash, 10, 1, valSet.Validators[i].Address, keys[i])
	}

	qc := &types.QuorumCertificate{
		BlockHash: blockHash,
		Round:     1,
		Votes:     votes,
	}

	power := qc.VotingPower(valSet)
	if power != 200 {
		t.Fatalf("expected power 200, got %d", power)
	}
}

// --- ValidatorSet ---

func TestValidatorSetQuorum(t *testing.T) {
	tests := []struct {
		n      int
		power  uint64
		quorum uint64
	}{
		{1, 100, 67},   // total=100, f=33, q=67
		{4, 100, 267},  // total=400, f=133, q=267
		{3, 100, 199},  // total=300, f=99, q=199
		{7, 100, 467},  // total=700, f=233, q=467
		{10, 100, 667}, // total=1000, f=333, q=667
	}

	for _, tt := range tests {
		vs, _ := makeTestValidatorSet(tt.n, tt.power)
		q := vs.Quorum()
		if q != tt.quorum {
			t.Errorf("n=%d power=%d: expected quorum %d, got %d", tt.n, tt.power, tt.quorum, q)
		}
	}
}

func TestValidatorSetGetProposerDeterministic(t *testing.T) {
	valSet, _ := makeTestValidatorSet(4, 100)

	p1 := valSet.GetProposer(10, 1)
	p2 := valSet.GetProposer(10, 1)
	if p1.Address != p2.Address {
		t.Fatal("GetProposer should be deterministic")
	}

	// Different round -> potentially different proposer.
	p3 := valSet.GetProposer(10, 2)
	// (10+1)%4=3, (10+2)%4=0 => should differ
	if p1.Address == p3.Address {
		// They could be the same if (h+r1)%n == (h+r2)%n, but with n=4, r1=1, r2=2 they differ
		t.Fatal("different rounds should produce different proposers")
	}
}

func TestValidatorSetGetProposerRotation(t *testing.T) {
	valSet, _ := makeTestValidatorSet(4, 100)

	// Check that we cycle through all validators.
	seen := make(map[types.Address]bool)
	for r := range uint64(4) {
		p := valSet.GetProposer(0, r)
		seen[p.Address] = true
	}
	if len(seen) != 4 {
		t.Fatalf("expected 4 distinct proposers over 4 rounds, got %d", len(seen))
	}
}

func TestValidatorSetGetByAddress(t *testing.T) {
	valSet, _ := makeTestValidatorSet(4, 100)

	v, ok := valSet.GetByAddress(valSet.Validators[2].Address)
	if !ok {
		t.Fatal("should find validator by address")
	}
	if v.VotingPower != 100 {
		t.Fatal("voting power mismatch")
	}

	_, ok = valSet.GetByAddress(makeTestAddress(99))
	if ok {
		t.Fatal("should not find unknown address")
	}
}

func TestValidatorSetHasQuorum(t *testing.T) {
	valSet, _ := makeTestValidatorSet(4, 100) // total=400, quorum=267

	if valSet.HasQuorum(266) {
		t.Fatal("266 < 267, should not have quorum")
	}
	if !valSet.HasQuorum(267) {
		t.Fatal("267 >= 267, should have quorum")
	}
	if !valSet.HasQuorum(400) {
		t.Fatal("400 >= 267, should have quorum")
	}
}

func TestNewValidatorSetRejectsEmpty(t *testing.T) {
	_, err := types.NewValidatorSet(nil)
	if err == nil {
		t.Fatal("should reject empty validator set")
	}
}

func TestNewValidatorSetRejectsZeroPower(t *testing.T) {
	_, err := types.NewValidatorSet([]types.Validator{
		{Address: makeTestAddress(1), VotingPower: 0},
	})
	if err == nil {
		t.Fatal("should reject zero voting power")
	}
}

// --- Helpers ---

func makeTestAddress(seed byte) types.Address {
	var addr types.Address
	for i := range addr {
		addr[i] = seed
	}
	return addr
}

type testKey struct {
	pub  crypto.PublicKey
	priv crypto.PrivateKey
}

func makeTestValidatorSet(n int, power uint64) (*types.ValidatorSet, []testKey) {
	validators := make([]types.Validator, n)
	keys := make([]testKey, n)

	for i := range n {
		pub, priv, _ := crypto.GenerateKeypair()
		addr := crypto.AddressFromPubKey(pub)

		keys[i] = testKey{pub: pub, priv: priv}
		validators[i] = types.Validator{
			Address:     addr,
			PublicKey:   crypto.PubKeyTo32(pub),
			VotingPower: power,
		}
	}

	vs, _ := types.NewValidatorSet(validators)
	return vs, keys
}

func makeSignedVote(blockHash types.Hash, height, round uint64, voterID types.Address, key testKey) types.Vote {
	vote := types.Vote{
		BlockHash: blockHash,
		Height:    height,
		Round:     round,
		VoterID:   voterID,
	}
	sig := crypto.Sign(key.priv, vote.SigningPayload())
	vote.Signature = crypto.SigTo64(sig)
	return vote
}
