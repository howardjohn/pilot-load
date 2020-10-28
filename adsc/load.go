package adsc

import (
	"time"
)

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
