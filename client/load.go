package client

import (
	"context"
	"log"

	"github.com/howardjohn/pilot-load/adsc"
)

func Connect(ctx context.Context, pilotAddress string, config *adsc.Config) error {
	log.Println("Connecting:", config.IP)
	con, err := adsc.Dial(pilotAddress, "", config)
	if err != nil {
		return err
	}
	log.Println("Connected:", config.IP)
	con.Watch()

	log.Println("Got Initial Update:", config.IP)
	for {
		select {
		case u := <-con.Updates:
			if u == "close" {
				log.Println("Closing:", config.IP)
				return nil
			}
		case <-ctx.Done():
			log.Println("Context closed, exiting stream")
			con.Close()
			return nil
		}
	}
}
