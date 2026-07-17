// Package eventbus is a transport-agnostic pub/sub abstraction for gateway
// events. The default implementation is NATS JetStream; the Bus interface is
// deliberately small so a Kafka or Redis Streams backend can be dropped in
// later without touching consumers.
package eventbus

import (
	"context"
	"time"
)

// Msg is a received message. Handlers must signal completion via the embedded
// ack methods (the Bus implementation does this automatically based on the
// handler's returned error, but the methods are exposed for advanced control).
type Msg interface {
	Data() []byte
	Subject() string
	Ack() error
	Nak() error
	NakWithDelay(d time.Duration) error
	Term() error
}

// Handler processes a single message. Returning nil acks the message; returning
// an error naks it for redelivery (up to the consumer's MaxDeliver).
type Handler func(ctx context.Context, msg Msg) error

// ConsumerSpec describes a durable consumer.
type ConsumerSpec struct {
	// Durable is the durable consumer name. Workers of the same type share a
	// durable name to load-balance; distinct services use distinct names.
	Durable string
	// FilterSubjects restricts delivery to matching subjects, e.g.
	// []string{"discord.events.MESSAGE_CREATE.>"}.
	FilterSubjects []string
	// AckWait is how long the server waits for an ack before redelivery.
	AckWait time.Duration
	// MaxDeliver caps delivery attempts (0 => server default).
	MaxDeliver int
	// MaxAckPending bounds in-flight unacked messages. Set to 1 for strict
	// per-consumer ordering (serializes throughput).
	MaxAckPending int
	// BackOff overrides AckWait with staged redelivery intervals.
	BackOff []time.Duration
}

// Subscription is an active consumer.
type Subscription interface {
	Stop()
}

// Bus is the pub/sub contract.
type Bus interface {
	// Publish sends data on subject. A non-empty dedupID enables JetStream
	// exactly-once ingestion within the stream's duplicate window.
	Publish(ctx context.Context, subject string, data []byte, dedupID string) error
	// Consume creates/updates a durable consumer and starts delivering to h.
	Consume(ctx context.Context, spec ConsumerSpec, h Handler) (Subscription, error)

	// PublishCore sends a fire-and-forget message on a core (non-JetStream)
	// subject. Used for the control plane (bot lifecycle, presence), which is
	// latest-wins with periodic reconciliation rather than a durable log.
	PublishCore(subject string, data []byte) error
	// SubscribeCore delivers core-NATS messages on subject to h. Delivery is
	// at-most-once and unordered; the caller must tolerate missed messages
	// (the control plane reconciles on an interval and on gateway hello).
	SubscribeCore(subject string, h func(data []byte)) (Subscription, error)

	// Close stops all subscriptions and releases the connection.
	Close() error
}
