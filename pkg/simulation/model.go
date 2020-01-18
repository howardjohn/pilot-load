package simulation

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"text/template"

	"golang.org/x/sync/errgroup"
)

type Runner func(ctx context.Context) error

func (r Runner) Append(o Runner) Runner {
	return func(ctx context.Context) error {
		g, c := errgroup.WithContext(ctx)
		g.Go(func() error {
			return r(c)
		})
		g.Go(func() error {
			return o(c)
		})

		return g.Wait()
	}
}

type Simulation interface {
	Run(Args) (Runner, error)
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
