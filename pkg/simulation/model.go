package simulation

import (
	"bytes"
	"fmt"
	"log"
	"text/template"

	"golang.org/x/sync/errgroup"

	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

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

type AggregateSimulation struct {
	sync  []model.Simulation
	async []model.Simulation
}

func (a AggregateSimulation) Cleanup(ctx model.Context) error {
	panic("implement me")
}

var _ model.Simulation = &AggregateSimulation{}

func NewAggregateSimulation(sync []model.Simulation, async []model.Simulation) model.Simulation {
	return &AggregateSimulation{sync, async}
}

func (a AggregateSimulation) Run(ctx model.Context) error {
	g, c := errgroup.WithContext(ctx)
	ctx = model.Context{c, ctx.Args, nil}
	for _, s := range a.sync {
		if err := s.Run(ctx); err != nil {
			return fmt.Errorf("aggregate sync: %v", err)
		}
	}
	for _, s := range a.async {
		s := s
		g.Go(func() error {
			return s.Run(ctx)
		})
	}
	return g.Wait()
}

func RunConfig(ctx model.Context, render func() string) (err error) {
	go func() {
		<-ctx.Done()
		if err := deleteConfig(render()); err != nil {
			log.Println("error during cleanup: ", err)
		}
	}()
	if err = applyConfig(render()); err != nil {
		return fmt.Errorf("failed to apply config: %v", err)
	}
	return nil
}
