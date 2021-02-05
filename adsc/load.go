package adsc

import (
	"fmt"
	"time"
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

	log("Got Initial Update: %v", config.IP)
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
	for {
		log("Connecting: %v", config.IP)
		con, err := Dial(pilotAddress, config)
		if err != nil {
			log("Error in ADS connection: %v", err)
			attempts++
			select {
			case <-config.Context.Done():
				log("Context closed, exiting stream")
				con.Close()
				return
			case <-time.After(time.Second * time.Duration(attempts)):
				log("Starting retry %v", attempts)
			}
			continue
		}

		log("Connected: %v", config.IP)
		con.Watch()

		log("Got Initial Update: %v", config.IP)
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
			case <-config.Context.Done():
				// We are really done now. Shut everything down and stop
				log("Context closed, exiting stream")
				con.Close()
				return
			}
		}
		log("Disconnected: %v", config.IP)
	}
}
