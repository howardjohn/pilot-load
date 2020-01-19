package simulation

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"
	"sync"
	"text/template"
	"time"

	"golang.org/x/sync/errgroup"
)

type Simulation interface {
	Run(ctx Context) error
}

type Context struct {
	context.Context
	args Args
}

var funcMap = map[string]interface{}{}

func render(yml string, spec interface{}) string {
	t, err := template.New(fmt.Sprintf("%T", spec)).Funcs(funcMap).Parse(yml)
	if err != nil {
		panic("failed to render template: " + err.Error())
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, spec); err != nil {
		panic("failed to render template: " + err.Error())
	}
	return buf.String()
}

func combineYaml(yml ...string) string {
	return strings.Join(yml, "---")
}

var chars = []rune("abcdefghijklmnopqrstuvwxyz")

func genUID() string {
	length := 8
	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.String()
}

var (
	ipMutex sync.Mutex
	nextIp  = net.ParseIP("10.0.0.10")
)

func getIp() string {
	ipMutex.Lock()
	defer ipMutex.Unlock()
	i := nextIp.To4()
	ret := i.String()
	v := uint(i[0])<<24 + uint(i[1])<<16 + uint(i[2])<<8 + uint(i[3])
	v += 1
	v3 := byte(v & 0xFF)
	v2 := byte((v >> 8) & 0xFF)
	v1 := byte((v >> 16) & 0xFF)
	v0 := byte((v >> 24) & 0xFF)
	nextIp = net.IPv4(v0, v1, v2, v3)
	return ret
}

type AggregateSimulation struct {
	simulations []Simulation
}

var _ Simulation = &AggregateSimulation{}

func NewAggregateSimulation(simulations []Simulation) Simulation {
	return &AggregateSimulation{simulations: simulations}
}

func (a AggregateSimulation) Run(ctx Context) error {
	g, c := errgroup.WithContext(ctx)
	ctx = Context{c, ctx.args}
	for _, s := range a.simulations {
		log.Println(fmt.Sprintf("running %T", s))
		g.Go(func() error {
			return s.Run(ctx)
		})
	}
	return g.Wait()
}

func RunConfig(ctx Context, render func() string) (err error) {
	for attempt := 0; attempt < 5; attempt++ {
		if err = applyConfig(render()); err != nil {
			if attempt == 4 {
				return fmt.Errorf("failed to apply config: %v", err)
			} else {
				time.Sleep(time.Second)
			}
		} else {
			break
		}
	}
	<-ctx.Done()
	defer func() {
		err = deleteConfig(render())
	}()
	return nil
}
