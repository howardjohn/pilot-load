package client

import (
	"context"
	"time"

	"istio.io/pkg/log"

	"github.com/howardjohn/pilot-load/adsc"
)

func Connect(ctx context.Context, pilotAddress string, config *adsc.Config) {
	attempts := 0
	for {
		log.Infof("Connecting: %v", config.IP)
		con, err := adsc.Dial(pilotAddress, "", config)
		if err != nil {
			log.Infof("Error in ADS connection: %v", err)
			attempts++
			select {
			case <-ctx.Done():
				log.Infof("Context closed, exiting stream")
				con.Close()
			case <-time.After(time.Second * time.Duration(attempts)):
				log.Infof("Starting retry %v")
			}
			continue
		}

		log.Infof("Connected: %v", config.IP)
		con.Watch()

		log.Infof("Got Initial Update: %v", config.IP)
		exit := false
		for !exit {
			select {
			case u := <-con.Updates:
				if u == "close" {
					// Close triggered. This may mean Pilot is just disconnecting, scaling, etc
					// Try the whole loop again
					log.Infof("Closing: %v", config.IP)
					exit = true
				}
			case <-ctx.Done():
				// We are really done now. Shut everything down and stop
				log.Infof("Context closed, exiting stream")
				con.Close()
				return
			}
		}
		log.Infof("Disconnected: %v", config.IP)
	}
}
