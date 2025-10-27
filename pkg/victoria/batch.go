package victoria

import (
	"sync"
	"time"

	"istio.io/istio/pkg/log"
)

type BatchReporter[T any] struct {
	address      string
	streamFields []string
	entryChan    chan T
	mu           sync.Mutex
	closeChan    chan struct{}
	wg           sync.WaitGroup
}

const (
	MAX_BATCH    = 500
	MAX_DURATION = time.Second * 5
)

func NewBatchReporter[T any](address string, streamFields []string) *BatchReporter[T] {
	br := &BatchReporter[T]{
		address:      address,
		streamFields: streamFields,
		entryChan:    make(chan T, 500),
		wg:           sync.WaitGroup{},
		closeChan:    make(chan struct{}),
	}
	br.wg.Add(1)
	go br.flusher()

	return br
}

func (br *BatchReporter[T]) Report(entries ...T) {
	for _, entry := range entries {
		select {
		case br.entryChan <- entry:
		case <-br.closeChan:
			return // or return an error if you prefer
		}
	}
}

func (br *BatchReporter[T]) Close() {
	close(br.closeChan)
	br.wg.Wait()
}

func (br *BatchReporter[T]) flusher() {
	defer br.wg.Done()

	ticker := time.NewTicker(MAX_DURATION)
	defer ticker.Stop()

	batch := make([]T, 0, MAX_BATCH)

	send := func() {
		br.send(batch)
		batch = batch[0:0]
		ticker.Reset(MAX_DURATION)
	}
	for {
		select {
		case entry := <-br.entryChan:
			batch = append(batch, entry)

			if len(batch) >= MAX_BATCH {
				send()
			}

		case <-ticker.C:
			if len(batch) > 0 {
				send()
			}

		case <-br.closeChan:
			// Drain any remaining entries in channel
			for {
				select {
				case entry := <-br.entryChan:
					batch = append(batch, entry)
				default:
					if len(batch) > 0 {
						send()
					}
					return
				}
			}
		}
	}
}

func (br *BatchReporter[T]) send(batch []T) {
	if err := Report(br.address, br.streamFields, batch); err != nil {
		log.Warnf("failed to report: %v", err)
	}
}
