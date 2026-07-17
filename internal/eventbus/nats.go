package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NATSConfig configures the JetStream connection + backing stream.
type NATSConfig struct {
	URL      string
	Stream   string
	Subjects []string      // stream subjects, e.g. []string{"discord.events.>"}
	MaxAge   time.Duration // 0 => 24h
	Replicas int           // 0 => 1
}

type natsBus struct {
	nc     *nats.Conn
	js     jetstream.JetStream
	stream string
	log    *slog.Logger

	mu   sync.Mutex
	subs []jetstream.ConsumeContext
}

// ConnectNATS dials NATS, initializes JetStream and ensures the event stream
// exists, returning a Bus.
func ConnectNATS(ctx context.Context, cfg NATSConfig, log *slog.Logger) (Bus, error) {
	nc, err := nats.Connect(cfg.URL,
		nats.Name("dia"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Warn("nats disconnected", "err", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			log.Info("nats reconnected", "url", c.ConnectedUrl())
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream init: %w", err)
	}

	b := &natsBus{nc: nc, js: js, stream: cfg.Stream, log: log}
	if err := b.ensureStream(ctx, cfg); err != nil {
		nc.Close()
		return nil, err
	}
	log.Info("connected to nats jetstream", "url", cfg.URL, "stream", cfg.Stream)
	return b, nil
}

func (b *natsBus) ensureStream(ctx context.Context, cfg NATSConfig) error {
	maxAge := cfg.MaxAge
	if maxAge == 0 {
		maxAge = 24 * time.Hour
	}
	replicas := cfg.Replicas
	if replicas == 0 {
		replicas = 1
	}
	_, err := b.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       cfg.Stream,
		Subjects:   cfg.Subjects,
		Storage:    jetstream.FileStorage,
		Retention:  jetstream.LimitsPolicy,
		Discard:    jetstream.DiscardOld,
		MaxAge:     maxAge,
		Duplicates: 2 * time.Minute,
		Replicas:   replicas,
	})
	if err != nil {
		return fmt.Errorf("ensure stream %q: %w", cfg.Stream, err)
	}
	return nil
}

func (b *natsBus) Publish(ctx context.Context, subject string, data []byte, dedupID string) error {
	var opts []jetstream.PublishOpt
	if dedupID != "" {
		opts = append(opts, jetstream.WithMsgID(dedupID))
	}
	if _, err := b.js.Publish(ctx, subject, data, opts...); err != nil {
		return fmt.Errorf("publish %q: %w", subject, err)
	}
	return nil
}

func (b *natsBus) Consume(ctx context.Context, spec ConsumerSpec, h Handler) (Subscription, error) {
	cc := jetstream.ConsumerConfig{
		Durable:        spec.Durable,
		AckPolicy:      jetstream.AckExplicitPolicy,
		FilterSubjects: spec.FilterSubjects,
		MaxDeliver:     spec.MaxDeliver,
		MaxAckPending:  spec.MaxAckPending,
	}
	if spec.AckWait > 0 {
		cc.AckWait = spec.AckWait
	}
	if len(spec.BackOff) > 0 {
		cc.BackOff = spec.BackOff
	}

	cons, err := b.js.CreateOrUpdateConsumer(ctx, b.stream, cc)
	if err != nil {
		return nil, fmt.Errorf("create consumer %q: %w", spec.Durable, err)
	}

	consumeCtx, err := cons.Consume(func(m jetstream.Msg) {
		// jetstream.Msg already satisfies eventbus.Msg.
		if herr := h(ctx, m); herr != nil {
			b.log.Warn("event handler error", "subject", m.Subject(), "consumer", spec.Durable, "err", herr)
			_ = m.Nak()
			return
		}
		_ = m.Ack()
	})
	if err != nil {
		return nil, fmt.Errorf("start consume %q: %w", spec.Durable, err)
	}

	b.mu.Lock()
	b.subs = append(b.subs, consumeCtx)
	b.mu.Unlock()

	b.log.Info("consuming", "consumer", spec.Durable, "filters", spec.FilterSubjects)
	return &natsSub{cc: consumeCtx}, nil
}

func (b *natsBus) PublishCore(subject string, data []byte) error {
	if err := b.nc.Publish(subject, data); err != nil {
		return fmt.Errorf("core publish %q: %w", subject, err)
	}
	return b.nc.Flush()
}

func (b *natsBus) SubscribeCore(subject string, h func(data []byte)) (Subscription, error) {
	sub, err := b.nc.Subscribe(subject, func(m *nats.Msg) { h(m.Data) })
	if err != nil {
		return nil, fmt.Errorf("core subscribe %q: %w", subject, err)
	}
	b.log.Info("control subscribed", "subject", subject)
	return &coreSub{sub: sub}, nil
}

type coreSub struct{ sub *nats.Subscription }

func (s *coreSub) Stop() { _ = s.sub.Unsubscribe() }

func (b *natsBus) Close() error {
	b.mu.Lock()
	for _, s := range b.subs {
		s.Stop()
	}
	b.subs = nil
	b.mu.Unlock()
	if err := b.nc.Drain(); err != nil {
		b.nc.Close()
		return err
	}
	return nil
}

type natsSub struct{ cc jetstream.ConsumeContext }

func (s *natsSub) Stop() { s.cc.Stop() }
