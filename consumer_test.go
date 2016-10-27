package nsq

import (
	"fmt"
	"strconv"
	"testing"
	"time"
)

func TestConsumer(t *testing.T) {
	for _, n := range []int{1, 10, 100, 1000} {
		count := n
		topic := fmt.Sprintf("test-consumer-%d", n)

		t.Run(topic, func(t *testing.T) {
			t.Parallel()
			nodes := make([]*Client, len(nsqd))

			// Create the clients used for publishing messages.
			for i, addr := range nsqd {
				nodes[i] = &Client{Address: addr}

				// stack cleanup callbacks
				defer func(i int) {
					nodes[i].DeleteTopic(topic)
				}(i)
			}

			// Publish messages to the NSQ nodes in a round robin fashion.
			for i := 0; i != count; i++ {
				node := nodes[i%len(nodes)]

				if err := node.Publish(topic, []byte(strconv.Itoa(i))); err != nil {
					t.Error(err)
					return
				}
			}

			// Each bucket should have a value of 1 after the test.
			buckets := make([]int, count)

			// Start the consumer which looks for nsq nodes from the given nsqlookup
			// addresses.
			consumer, _ := StartConsumer(ConsumerConfig{
				Topic:       topic,
				Channel:     "buckets",
				Lookup:      nsqlookup,
				MaxInFlight: count / 10,
			})
			defer consumer.Stop()

			deadline := time.NewTimer(10 * time.Second)
			defer deadline.Stop()

			// Consume messages until we've (hopefully) got them all.
			for i := 0; i != count; i++ {
				select {
				case msg := <-consumer.Messages():
					if b, err := strconv.Atoi(string(msg.Body)); err != nil {
						t.Error("invalid message:", msg)
					} else {
						buckets[b]++
					}
					msg.Finish()

				case <-deadline.C:
					t.Error("timeout")
					return
				}
			}

			// Check that we've got the expected results.
			for i, b := range buckets {
				if b != 1 {
					t.Errorf("bucket at index %d has value %d", i, b)
				}
			}
		})
	}
}
