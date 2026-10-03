package queue

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"
	"uuid"

	drmq "github.com/samuel025/DRMQ/drmq-go-client"
	"github.com/samuel025/url-shortner-go/internal/database"
)

type ClickEvent struct {
	URLID     uuid.UUID `json:"url_id"`
	Timestamp time.Time `json:"timestamp"`
}

type Client struct {
	producer *drmq.Producer
	consumer *drmq.Consumer
	topic    string
	models   database.Models
	logger   *slog.Logger
	stopOnce sync.Once
	stopChan chan struct{}
	doneChan chan struct{}
}

func New(bootstrapServers, topic, groupID string, models database.Models, logger *slog.Logger) (*Client, error) {
	prod, err := drmq.NewProducer(drmq.ProducerConfig{
		BootstrapServers: bootstrapServers,
		BatchSizeBytes:   16384,
		LingerMs:         5,
		MaxInflight:      5,
		Logger:           logger,
	})
	if err != nil {
		return nil, err
	}
	if err := prod.Connect(); err != nil {
		return nil, err
	}

	cons, err := drmq.NewConsumer(drmq.ConsumerConfig{
		BootstrapServers: bootstrapServers,
		ConsumerGroup:    groupID,
		AutoCommit:       false,
		Logger:           logger,
	})
	if err != nil {
		prod.Close()
		return nil, err
	}
	if err := cons.Connect(); err != nil {
		prod.Close()
		return nil, err
	}
	if err := cons.Subscribe(topic); err != nil {
		_ = cons.Close()
		prod.Close()
		return nil, err
	}

	c := &Client{
		producer: prod,
		consumer: cons,
		topic:    topic,
		models:   models,
		logger:   logger,
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}

	go c.startWorker()

	return c, nil
}

func (c *Client) startWorker() {
	defer close(c.doneChan)

	for {
		select {
		case <-c.stopChan:
			return
		default:
			msgs, err := c.consumer.PollWithOptions(200, 100)
			if err != nil {
				select {
				case <-c.stopChan:
					return
				case <-time.After(100 * time.Millisecond):
					continue
				}
			}

			if len(msgs) == 0 {
				select {
				case <-c.stopChan:
					return
				case <-time.After(200 * time.Millisecond):
					continue
				}
			}

			counts := make(map[uuid.UUID]int)
			var lastOffset int64 = -1

			for _, msg := range msgs {
				var event ClickEvent
				if err := json.Unmarshal(msg.Payload, &event); err == nil && event.URLID != (uuid.UUID{}) {
					counts[event.URLID]++
				}
				if msg.Offset > lastOffset {
					lastOffset = msg.Offset
				}
			}

			for urlID, count := range counts {
				if err := c.models.URLs.IncrementClickCountBy(urlID, count); err != nil {
					c.logger.Error("Failed to increment click count in worker", "url_id", urlID, "count", count, "error", err)
				}
			}

			if lastOffset >= 0 {
				_ = c.consumer.Commit(c.topic, lastOffset)
			}
		}
	}
}

func (c *Client) PublishClick(urlID uuid.UUID) error {
	if c == nil || c.producer == nil {
		return nil
	}

	event := ClickEvent{
		URLID:     urlID,
		Timestamp: time.Now().UTC(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	future := c.producer.SendWithKey(c.topic, data, urlID.String())
	go func(id uuid.UUID, f *drmq.SendFuture) {
		if _, err := f.Get(); err != nil {
			if c.logger != nil {
				c.logger.Error("DRMQ publish failed, falling back to direct DB increment", "url_id", id, "error", err)
			}
			if dbErr := c.models.URLs.IncrementClickCount(id); dbErr != nil && c.logger != nil {
				c.logger.Error("Fallback DB increment failed", "url_id", id, "error", dbErr)
			}
		}
	}(urlID, future)

	return nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}

	c.stopOnce.Do(func() {
		close(c.stopChan)
		<-c.doneChan
		if c.consumer != nil {
			_ = c.consumer.Close()
		}
		if c.producer != nil {
			c.producer.Close()
		}
	})

	return nil
}
