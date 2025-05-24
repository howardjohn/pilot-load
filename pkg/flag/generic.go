package flag

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
)

type Parseable interface{}

func Register[T Parseable](flags *pflag.FlagSet, val *T, name string, description string) {
	RegisterShort[T](flags, val, name, "", description)
}

func RegisterShort[T Parseable](flags *pflag.FlagSet, val *T, name, short string, description string) {
	defaultValue := *val
	switch d := any(defaultValue).(type) {
	case string:
		flags.StringVarP(any(val).(*string), name, short, d, description)
	case bool:
		flags.BoolVarP(any(val).(*bool), name, short, d, description)
	case int:
		flags.IntVarP(any(val).(*int), name, short, d, description)
	case []string:
		flags.StringSliceVarP(any(val).(*[]string), name, short, d, description)
	case time.Duration:
		flags.DurationVarP(any(val).(*time.Duration), name, short, d, description)
	default:
		panic(fmt.Sprintf("unknown type %T", d))
	}
}
