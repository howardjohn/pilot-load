package client

import (
	"fmt"
	"sync"
	"time"

	"istio.io/istio/pkg/adsc"
)

func makeADSC(addr string, client int, prefix int) error {
	ip := fmt.Sprintf("%d.0.%d.%d", prefix, client/256, client%256)
	fmt.Println("Connecting:", ip)
	con, err := adsc.Dial(addr, "", &adsc.Config{
		IP: ip,
	})
	if err != nil {
		return err
	}
	fmt.Println("Connected:", ip)
	con.Watch()
	fmt.Println("Got Initial Update:", ip)
	return nil
}

func RunLoad(pilotAddress string, clients int, prefix int) error {
	wg := sync.WaitGroup{}
	wg.Add(1)
	for cur := 0; cur < clients; cur++ {
		go makeADSC(pilotAddress, cur, prefix)
		time.Sleep(time.Millisecond * 100)
	}
	wg.Wait()
	return nil
}
