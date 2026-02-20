package p2p

import (
	"context"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/echenim/Bedrock/controlplane/internal/consensus"
	"github.com/echenim/Bedrock/controlplane/internal/types"
	"go.uber.org/zap"
)

// Compile-time check that P2PTransport implements consensus.Transport.
var _ consensus.Transport = (*P2PTransport)(nil)

// MessageSubscription holds channels for receiving decoded consensus messages.
type MessageSubscription struct {
	Proposals chan *types.Proposal
	Votes     chan *types.Vote
	Timeouts  chan *types.TimeoutMessage
}

// P2PTransport implements consensus.Transport over GossipSub.
type P2PTransport struct {
	host    *Host
	valSet  *types.ValidatorSet
	metrics *Metrics
	logger  *zap.Logger

	mu   sync.RWMutex
	subs []MessageSubscription

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewP2PTransport creates a transport that bridges GossipSub and the consensus engine.
func NewP2PTransport(host *Host, valSet *types.ValidatorSet, logger *zap.Logger) *P2PTransport {
	if logger == nil {
		logger = zap.NewNop()
	}
	metrics := host.metrics
	if metrics == nil {
		metrics = NopMetrics()
	}
	return &P2PTransport{
		host:    host,
		valSet:  valSet,
		metrics: metrics,
		logger:  logger,
	}
}

// BroadcastProposal publishes a proposal to the consensus topic.
func (t *P2PTransport) BroadcastProposal(proposal *types.Proposal) error {
	data, err := EncodeProposal(proposal)
	if err != nil {
		return err
	}
	t.metrics.MessagesSent.WithLabelValues("proposal").Inc()
	return t.host.gossip.Publish(context.TODO(), TopicConsensus, data)
}

// BroadcastVote publishes a vote to the consensus topic.
func (t *P2PTransport) BroadcastVote(vote *types.Vote) error {
	data, err := EncodeVote(vote)
	if err != nil {
		return err
	}
	t.metrics.MessagesSent.WithLabelValues("vote").Inc()
	return t.host.gossip.Publish(context.TODO(), TopicConsensus, data)
}

// BroadcastTimeout publishes a timeout message to the consensus topic.
func (t *P2PTransport) BroadcastTimeout(msg *types.TimeoutMessage) error {
	data, err := EncodeTimeout(msg)
	if err != nil {
		return err
	}
	t.metrics.MessagesSent.WithLabelValues("timeout").Inc()
	return t.host.gossip.Publish(context.TODO(), TopicConsensus, data)
}

// Subscribe returns a MessageSubscription for receiving decoded consensus messages.
func (t *P2PTransport) Subscribe() MessageSubscription {
	sub := MessageSubscription{
		Proposals: make(chan *types.Proposal, 16),
		Votes:     make(chan *types.Vote, 64),
		Timeouts:  make(chan *types.TimeoutMessage, 16),
	}
	t.mu.Lock()
	t.subs = append(t.subs, sub)
	t.mu.Unlock()
	return sub
}

// UpdateValidatorSet atomically updates the validator set used for message validation.
func (t *P2PTransport) UpdateValidatorSet(valSet *types.ValidatorSet) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.valSet = valSet
}

// Start begins reading from the GossipSub consensus subscription, validating
// messages (protobuf decode, signature verification, validator membership),
// and dispatching to subscriber channels.
func (t *P2PTransport) Start(ctx context.Context) error {
	sub, err := t.host.gossip.Subscribe(TopicConsensus)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.readLoop(ctx, sub)
	}()

	return nil
}

// Stop shuts down the transport read loop.
func (t *P2PTransport) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
	t.wg.Wait()
}

func (t *P2PTransport) readLoop(ctx context.Context, sub *pubsub.Subscription) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			t.logger.Warn("gossip subscription error", zap.Error(err))
			return
		}

		// Skip our own messages.
		if msg.ReceivedFrom == t.host.ID() {
			continue
		}

		t.handleMessage(msg.Data, msg.ReceivedFrom)
	}
}

func (t *P2PTransport) handleMessage(data []byte, from interface{ String() string }) {
	msgType, decoded, err := DecodeMessage(data)
	if err != nil {
		t.metrics.MessagesRejected.WithLabelValues("decode_error").Inc()
		t.logger.Debug("failed to decode message", zap.Error(err))
		return
	}

	t.mu.RLock()
	valSet := t.valSet
	t.mu.RUnlock()

	switch msgType {
	case MsgProposal:
		proposal := decoded.(*types.Proposal)
		t.metrics.MessagesReceived.WithLabelValues("proposal").Inc()
		t.dispatchProposal(proposal)

	case MsgVote:
		vote := decoded.(*types.Vote)
		// Two-stage validation: voter must be in validator set with valid signature.
		if valSet != nil {
			val, ok := valSet.GetByAddress(vote.VoterID)
			if !ok {
				t.metrics.MessagesRejected.WithLabelValues("unknown_validator").Inc()
				return
			}
			if !vote.Verify(val.PublicKey) {
				t.metrics.MessagesRejected.WithLabelValues("invalid_signature").Inc()
				return
			}
		}
		t.metrics.MessagesReceived.WithLabelValues("vote").Inc()
		t.dispatchVote(vote)

	case MsgTimeout:
		tm := decoded.(*types.TimeoutMessage)
		t.metrics.MessagesReceived.WithLabelValues("timeout").Inc()
		t.dispatchTimeout(tm)
	}
}

func (t *P2PTransport) dispatchProposal(p *types.Proposal) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, sub := range t.subs {
		select {
		case sub.Proposals <- p:
		default:
			t.logger.Warn("proposal subscriber channel full, dropping")
		}
	}
}

func (t *P2PTransport) dispatchVote(v *types.Vote) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, sub := range t.subs {
		select {
		case sub.Votes <- v:
		default:
			t.logger.Warn("vote subscriber channel full, dropping")
		}
	}
}

func (t *P2PTransport) dispatchTimeout(tm *types.TimeoutMessage) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, sub := range t.subs {
		select {
		case sub.Timeouts <- tm:
		default:
			t.logger.Warn("timeout subscriber channel full, dropping")
		}
	}
}
