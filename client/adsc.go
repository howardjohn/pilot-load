package client

import (
	"fmt"
	"sync"

	"istio.io/istio/pkg/adsc"
)

func makeADSC(addr string, client int) error {
	ip := fmt.Sprintf("127.0.%d.%d", client/256, client%256)
	fmt.Println("Connecting:", ip)
	con, err := adsc.Dial(addr, "", &adsc.Config{
		IP: ip,
	})
	if err != nil {
		return err
	}
	fmt.Println("Connected:", ip)
	con.Watch()
	return nil
}

func RunLoad(pilotAddress string, clients int) error {
	wg := sync.WaitGroup{}
	wg.Add(1)
	for cur := 0; cur < clients; cur++ {
		go makeADSC(pilotAddress, cur)
	}
	wg.Wait()
	return nil
}
