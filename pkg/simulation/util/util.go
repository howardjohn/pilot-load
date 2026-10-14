package util

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
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

func StringDefault(s string, def string) string {
	if s != "" {
		return s
	}
	return def
}

func IsDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func ContextSleep(ctx context.Context, dur time.Duration) {
	select {
	case <-time.After(dur):
	case <-ctx.Done():
	}
}

func GenUIDOrStableIdentifier(stable bool, index int, replica int) string {
	if !stable {
		return GenUID()
	}
	return fmt.Sprintf("%d-%d", index, replica)
}

func GenUID() string {
	length := 8
	var b strings.Builder
	for i := 0; i < length; i++ {
		b.WriteRune(chars[rand.Intn(len(chars))])
	}
	return b.String()
}

// DefaultIPRange is the range GetIP hands out addresses from: starting at
// 10.0.0.10, within 10.0.0.0/8.
const DefaultIPRange = "10.0.0.10/8"

var (
	ipMutex sync.Mutex
	nextIp  = net.ParseIP("10.0.0.10")
	ipRange *net.IPNet
)

// SetIPRange sets the range GetIP hands out addresses from, in CIDR
// notation: the address is the first one handed out and the prefix bounds
// the range (10.64.0.10/16 hands out 10.64.0.10 to 10.64.255.255). Separate
// pilot-load processes against one cluster can use disjoint ranges this way,
// so the pods, nodes and workload entries they simulate never share an
// address.
func SetIPRange(cidr string) error {
	ip, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("invalid IP range %q: %v", cidr, err)
	}
	if ip.To4() == nil {
		return fmt.Errorf("invalid IP range %q: only IPv4 is supported", cidr)
	}
	ipMutex.Lock()
	defer ipMutex.Unlock()
	nextIp = ip.To4()
	ipRange = ipNet
	return nil
}

// GetIP returns the next simulated IP. It panics when the range set by
// SetIPRange is exhausted rather than handing out an address outside it.
func GetIP() string {
	ipMutex.Lock()
	defer ipMutex.Unlock()
	i := nextIp.To4()
	if ipRange != nil && !ipRange.Contains(i) {
		panic(fmt.Sprintf("simulated IP range %v exhausted", ipRange))
	}
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
