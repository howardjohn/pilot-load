package victoriapush

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/pflag"

	"github.com/howardjohn/pilot-load/pkg/flag"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
	"github.com/howardjohn/pilot-load/pkg/victoria"
)

// TODO: allow customization
type LogIn struct {
	Latency   float64 `json:"latency"`
	Timestamp int64   `json:"timestamp"`
	Thread    int     `json:"thread"`
	Iter      int     `json:"iter"`
	Details   string  `json:"details"`
}

type Log struct {
	Message   string  `json:"_msg"`
	Time      int64   `json:"_time"`
	Value     float64 `json:"value"`
	Stream    string  `json:"stream"`
	Substream string  `json:"subStream,omitempty"`
}

func Command(f *pflag.FlagSet) flag.Command {
	time.Now().UnixNano()
	var address string
	stream := "victoriapush"
	var subStream string
	flag.RegisterShort(f, &address, "victoria-address", "v", "victorialogs address")
	flag.RegisterShort(f, &stream, "stream", "s", "victorialogs stream identifier")
	flag.Register(f, &subStream, "sub-stream", "victorialogs sub stream identifier")
	return flag.Command{
		Name:        "victoriapush",
		Description: "push logs to victoria logs",
		Details:     "",
		Build: func(args *model.Args) (model.DebuggableSimulation, error) {
			sid := []string{"stream"}
			if subStream != "" {
				sid = append(sid, "subStream")
			}
			p := Pusher{Batcher: victoria.NewBatchReporter[Log](address, sid), Stream: stream, SubStream: subStream}
			return p, nil
		},
	}
}

type Pusher struct {
	Batcher   *victoria.BatchReporter[Log]
	Stream    string
	SubStream string
}

func (p Pusher) Run(ctx model.Context) error {
	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Bytes()

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		var record LogIn
		if err := json.Unmarshal(line, &record); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing line: %v\n", err)
			continue
		}
		out := Log{
			Message:   record.Details,
			Time:      record.Timestamp,
			Value:     record.Latency,
			Stream:    p.Stream,
			Substream: p.SubStream,
		}
		p.Batcher.Report(out)
	}
	ctx.Cancel()

	return scanner.Err()
}

func (p Pusher) Cleanup(ctx model.Context) error {
	p.Batcher.Close()
	return nil
}

func (p Pusher) GetConfig() any {
	return "global"
}
