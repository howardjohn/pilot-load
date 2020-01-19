package client

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/howardjohn/pilot-load/adsc"
)

func makeADSC(addr string, client int, prefix int, verbose bool) error {
	ip := fmt.Sprintf("%d.0.%d.%d", prefix, client/256, client%256)
	log.Println("Connecting:", ip)
	con, err := adsc.Dial(addr, "", &adsc.Config{
		IP: ip,
		Meta: map[string]interface{}{
			"ISTIO_VERSION": "1.9.0",
		},
		Verbose: verbose,
	})
	if err != nil {
		return err
	}
	log.Println("Connected:", ip)
	con.Watch()
	log.Println("Got Initial Update:", ip)
	for {
		u := <-con.Updates
		log.Println("Got update: ", u, " for ", ip)
		if u == "close" {
			log.Println("Closing:", ip)
			return nil
		}
	}
}

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
			log.Println("Got update: ", u, " for ", config.IP)
			if u == "close" {
				log.Println("Closing:", config.IP)
				return nil
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func RunLoad(pilotAddress string, clients int, prefix int, verbose bool) error {
	wg := sync.WaitGroup{}
	for cur := 0; cur < clients; cur++ {
		wg.Add(1)
		go func() {
			for {
				err := makeADSC(pilotAddress, cur, prefix, verbose)
				if err != nil {
					break
				}
				log.Println("connecton ended")
			}
			wg.Done()
		}()
		time.Sleep(time.Millisecond * 100)
	}
	wg.Wait()
	return nil
}
