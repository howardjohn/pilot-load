package security

import (
	"fmt"
	"sync"
	"time"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"go.uber.org/atomic"
)

// map of SAN to KeyPair. Use to avoid repetitive CSR creations
var cachedKeys sync.Map

// map of SAN to jwt token. Used to avoid repetitive calls
var cachedTokens sync.Map

// Cache the root cert
var rootCert atomic.String

type KeyPair struct {
	KeyPEM []byte
	CsrPEM []byte
}

func san(ns, sa string) string {
	return fmt.Sprintf("spiffe://%s/ns/%s/sa/%s", "cluster.local", ns, sa)
}

type token struct {
	token      string
	expiration time.Time
}

func GetServiceAccountToken(c *kube.Client, aud, ns, sa string) (string, error) {
	san := san(ns, sa)

	if got, f := cachedTokens.Load(san); f {
		t := got.(token)
		if !t.expiration.After(time.Now().Add(-time.Minute)) {
			return t.token, nil
		}
		// Otherwise, its expired, load a new one
	}

	t, exp, err := c.CreateServiceAccountToken(aud, ns, sa)
	if err != nil {
		return "", err
	}
	cachedTokens.Store(san, token{t, exp})
	return t, nil
}
