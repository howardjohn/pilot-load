package client

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/howardjohn/pilot-load/adsc"
)

func Connect(ctx context.Context, pilotAddress string, config *adsc.Config) error {
	failures := 0
	for {
		log.Println("Connecting:", config.IP)
		con, err := adsc.Dial(pilotAddress, "", config)
		if err != nil {
			log.Println("Error in ADS connection", err)
			failures++
			if failures > 10 {
				return fmt.Errorf("exceeded max errors: %v", err)
			}
			time.Sleep(time.Second * 1)
			continue
		}
		log.Println("Connected:", config.IP)
		con.Watch()

		log.Println("Got Initial Update:", config.IP)
		func() {
			for {
				select {
				case u := <-con.Updates:
					if u == "close" {
						log.Println("Closing:", config.IP)
					}
					return
				case <-ctx.Done():
					log.Println("Context closed, exiting stream")
					con.Close()
					return
				}
			}
		}()
		log.Println("Disconnected:", config.IP)
	}
}
