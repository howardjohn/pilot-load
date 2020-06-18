package util

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
)

func AddError(e1, e2 error) error {
	if e1 == nil {
		return e2
	}
	if e2 == nil {
		return e1
	}
	return fmt.Errorf("%v and %v", e1, e2)
}

var chars = []rune("abcdefghijklmnopqrstuvwxyz")

func StringPointer(s string) *string {
	return &s
}

func BoolPointer(b bool) *bool {
	return &b
}

func IsDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func GenUID() string {
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

func GetIP() string {
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
