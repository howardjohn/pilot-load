package adsc

import (
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
)

func Fetch(pilotAddress string, config *Config) (*Responses, error) {
	log := func(template string, args ...interface{}) {
		a := []interface{}{"%v: " + template, config.Workload}
		a = append(a, args...)
		scope.Infof(a...)
	}
	watchAll := map[string]struct{}{"cds": {}, "eds": {}, "rds": {}, "lds": {}}
	log("Connecting: %v", config.IP)
	con, err := Dial(pilotAddress, config)
	if err != nil {
		return nil, fmt.Errorf("ADS connection: %v", err)
	}

	log("Connected: %v", config.IP)
	con.Watch()

	exit := false
	for !exit {
		select {
		case u := <-con.Updates:
			if u == "close" {
				// Close triggered. This may mean Pilot is just disconnecting, scaling, etc
				// Try the whole loop again
				log("Closing: %v", config.IP)
				exit = true
			}
			delete(watchAll, u)
			if len(watchAll) == 0 {
				log("Done: %v", config.IP)
				exit = true
			}
		case <-config.Context.Done():
			// We are really done now. Shut everything down and stop
			con.Close()
			return nil, fmt.Errorf("context closed")
		}
	}
	con.conn.Close()
	con.mutex.Lock()
	defer con.mutex.Unlock()
	return &con.Responses, nil
}

func Connect(pilotAddress string, config *Config) {
	attempts := 0
	log := func(template string, args ...interface{}) {
		a := []interface{}{"%v: " + template, config.Workload}
		a = append(a, args...)
		scope.Infof(a...)
	}
	// Follow envoy defaults https://github.com/envoyproxy/envoy/blob/v1.12.1/source/common/config/grpc_stream.h#L40-L43
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 0
	b.MaxInterval = time.Second * 30
	b.InitialInterval = time.Millisecond * 500
	for {
		t0 := time.Now()
		log("Connecting: %v", config.IP)
		con, err := Dial(pilotAddress, config)
		if err != nil {
			log("Error in ADS connection: %v", err)
			attempts++
			bo := b.NextBackOff()
			select {
			case <-config.Context.Done():
				log("Context closed, exiting stream")
				con.Close()
				return
			case <-time.After(bo):
				log("Starting retry %v after %v", attempts, bo)
			}
			continue
		}

		log("Connected: %v in %v", config.IP, time.Since(t0))
		con.Watch()

		update := false
		exit := false
		for !exit {
			select {
			case u := <-con.Updates:
				if u == "close" {
					// Close triggered. This may mean Pilot is just disconnecting, scaling, etc
					// Try the whole loop again
					log("Closing: %v", config.IP)
					exit = true
				} else if !update {
					update = true
					b.Reset()
					log("Got Initial Update: %v for %v in %v", config.IP, u, time.Since(t0))
				}
			case <-config.Context.Done():
				// We are really done now. Shut everything down and stop
				log("Context closed, exiting stream")
				con.Close()
				return
			}
		}
		bo := b.NextBackOff()
		log("Disconnected: %v, retrying in %v", config.IP, bo)
		select {
		case <-config.Context.Done():
			log("Context closed, exiting stream")
			con.Close()
			return
		case <-time.After(bo):
		}
	}
}
