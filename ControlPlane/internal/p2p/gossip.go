package p2p

import (
	"context"
	"fmt"
	"sync"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.uber.org/zap"
)

// Topic names for the gossip network.
const (
	TopicConsensus = "/bedrock/consensus/v1"
	TopicMempool   = "/bedrock/mempool/v1"
	TopicSync      = "/bedrock/sync/v1"
)

// GossipManager manages GossipSub topics and subscriptions.
type GossipManager struct {
	ps          *pubsub.PubSub
	host        host.Host
	scoring     *PeerScoring
	rateLimiter *RateLimiter
	logger      *zap.Logger

	mu     sync.RWMutex
	topics map[string]*pubsub.Topic
	subs   map[string]*pubsub.Subscription
}

// NewGossipManager creates a GossipSub instance with integrated peer scoring
// and flood publishing for consensus delivery.
func NewGossipManager(ctx context.Context, h host.Host, scoring *PeerScoring, rateLimiter *RateLimiter, logger *zap.Logger) (*GossipManager, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	opts := []pubsub.Option{
		pubsub.WithFloodPublish(true),
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
	}

	ps, err := pubsub.NewGossipSub(ctx, h, opts...)
	if err != nil {
		return nil, fmt.Errorf("p2p: create gossipsub: %w", err)
	}

	gm := &GossipManager{
		ps:          ps,
		host:        h,
		scoring:     scoring,
		rateLimiter: rateLimiter,
		logger:      logger,
		topics:      make(map[string]*pubsub.Topic),
		subs:        make(map[string]*pubsub.Subscription),
	}

	return gm, nil
}

// JoinTopic joins a GossipSub topic and stores the handle.
func (gm *GossipManager) JoinTopic(topicName string) (*pubsub.Topic, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if t, ok := gm.topics[topicName]; ok {
		return t, nil
	}

	topic, err := gm.ps.Join(topicName)
	if err != nil {
		return nil, fmt.Errorf("p2p: join topic %s: %w", topicName, err)
	}

	gm.topics[topicName] = topic
	return topic, nil
}

// Subscribe subscribes to a topic and returns the subscription.
func (gm *GossipManager) Subscribe(topicName string) (*pubsub.Subscription, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if sub, ok := gm.subs[topicName]; ok {
		return sub, nil
	}

	topic, ok := gm.topics[topicName]
	if !ok {
		return nil, fmt.Errorf("p2p: topic %s not joined", topicName)
	}

	sub, err := topic.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("p2p: subscribe to %s: %w", topicName, err)
	}

	gm.subs[topicName] = sub
	return sub, nil
}

// Publish publishes data to the named topic.
func (gm *GossipManager) Publish(ctx context.Context, topicName string, data []byte) error {
	gm.mu.RLock()
	topic, ok := gm.topics[topicName]
	gm.mu.RUnlock()

	if !ok {
		return fmt.Errorf("p2p: topic %s not joined", topicName)
	}

	return topic.Publish(ctx, data)
}

// RegisterConsensusValidator registers a lightweight GossipSub message validator
// for the consensus topic. This is the fast first-stage validation:
// rate limit check, ban check, and size check.
func (gm *GossipManager) RegisterConsensusValidator() error {
	return gm.ps.RegisterTopicValidator(TopicConsensus, func(ctx context.Context, from peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
		// Reject from banned peers.
		if gm.scoring != nil && gm.scoring.IsBanned(from) {
			return pubsub.ValidationReject
		}

		// Check message size.
		if len(msg.Data) == 0 || len(msg.Data) > MaxMessageSize {
			if gm.scoring != nil {
				gm.scoring.RecordInvalidMessage(from, "oversize_message")
			}
			return pubsub.ValidationReject
		}

		// Rate limit check.
		if gm.rateLimiter != nil {
			// Peek at the type byte for type-specific rate limiting.
			msgType := MessageType(msg.Data[0])
			if !gm.rateLimiter.Allow(from, msgType) {
				return pubsub.ValidationIgnore
			}
		}

		return pubsub.ValidationAccept
	})
}

// Close closes all subscriptions and topics.
func (gm *GossipManager) Close() {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	for name, sub := range gm.subs {
		sub.Cancel()
		delete(gm.subs, name)
	}
	for name, topic := range gm.topics {
		topic.Close()
		delete(gm.topics, name)
	}
}
