package eventbus

import (
	"context"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"sort"
	"strings"
	"time"
)

type broker struct {
	nc    *nats.Conn
	js    jetstream.JetStream
	cfg   Config
	epoch string
}

func connect(c Config, epoch, name string) (*broker, error) {
	opts := []nats.Option{nats.Name("InfraPro-" + c.Environment + "-" + name), nats.CustomInboxPrefix("ipreply." + c.Environment + "." + name), nats.Timeout(2 * time.Second), nats.ReconnectWait(time.Second), nats.MaxReconnects(-1), nats.ReconnectBufSize(0)}
	if c.CAFile != "" {
		opts = append(opts, nats.RootCAs(c.CAFile), nats.ClientCert(c.CertFile, c.KeyFile))
	}
	if c.CredentialsFile != "" {
		opts = append(opts, nats.UserCredentials(c.CredentialsFile))
	}
	nc, e := nats.Connect(c.NATSURL, opts...)
	if e != nil {
		return nil, errors.New("broker unavailable")
	}
	js, e := jetstream.New(nc)
	if e != nil {
		nc.Close()
		return nil, e
	}
	return &broker{nc, js, c, epoch}, nil
}
func (b *broker) subject(topic string) string {
	return "infrapro." + b.cfg.Environment + "-" + b.epoch + ".v1." + topic
}
func (b *broker) stream(f string) string {
	return "IP_" + b.cfg.Environment + "_" + strings.ReplaceAll(b.epoch, "-", "") + "_" + f
}
func advisorySubject(stream, consumer string) string {
	return "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES." + stream + "." + consumer
}

func (b *broker) provision(ctx context.Context) error {
	sizes := map[string]int64{"ingestion": b.cfg.IngestionBytes, "monitoring": b.cfg.MonitoringBytes, "auxiliary": b.cfg.AuxiliaryBytes}
	for _, f := range families {
		topicSet := map[string]bool{}
		for consumer, t := range consumerTopics {
			if family(t) == f {
				topicSet[b.subject(t)] = true
				topicSet[advisorySubject(b.stream(f), consumer)] = true
			}
		}
		var subjects []string
		for t := range topicSet {
			subjects = append(subjects, t)
		}
		sort.Strings(subjects)
		_, err := b.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{Name: b.stream(f), Subjects: subjects, Storage: jetstream.FileStorage, Retention: jetstream.LimitsPolicy, Discard: jetstream.DiscardNew, Replicas: b.cfg.Replicas, MaxBytes: sizes[f], MaxAge: time.Duration(b.cfg.RetentionSeconds) * time.Second, Duplicates: time.Duration(b.cfg.DedupSeconds) * time.Second, MaxMsgSize: MaxBytes})
		if err != nil {
			return err
		}
	}
	for consumer, topic := range consumerTopics {
		stream := b.stream(family(topic))
		cfg := jetstream.ConsumerConfig{Durable: consumer, FilterSubject: b.subject(topic), AckPolicy: jetstream.AckExplicitPolicy, AckWait: ms(b.cfg.AckWaitMS), MaxDeliver: b.cfg.MaxDeliver, MaxAckPending: b.cfg.MaxAckPending, MaxRequestBatch: b.cfg.Batch, Replicas: b.cfg.Replicas}
		if _, err := b.js.CreateOrUpdateConsumer(ctx, stream, cfg); err != nil {
			return err
		}
		// The exhaustion notice must outlive the process that missed the last
		// delivery. Store only these broker-generated notices, in the same
		// bounded family stream. A separate pull cursor acknowledges PG evidence.
		cfg.Durable = consumer + "-advisories"
		cfg.FilterSubject = advisorySubject(stream, consumer)
		cfg.MaxDeliver = -1
		if _, err := b.js.CreateOrUpdateConsumer(ctx, stream, cfg); err != nil {
			return err
		}
	}
	return nil
}
func Provision(ctx context.Context, c Config, epoch string) error {
	if !validID(epoch) {
		return errEpoch
	}
	b, e := connect(c, epoch, "admin")
	if e != nil {
		return e
	}
	defer b.nc.Close()
	return b.provision(ctx)
}
