package security

import (
	"fmt"
	"sync"

	"go.uber.org/atomic"

	"github.com/howardjohn/pilot-load/pkg/kube"

	pkiutil "istio.io/istio/security/pkg/pki/util"
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

func GetRootCert(c *kube.Client) (string, error) {
	if cert := rootCert.Load(); cert != "" {
		return cert, nil
	}
	cert, err := c.FetchRootCert()
	if err != nil {
		return "", err
	}
	rootCert.Store(cert)
	return cert, nil
}

func GetServiceAccountToken(c *kube.Client, ns, sa string) (string, error) {
	san := san(ns, sa)

	if got, f := cachedTokens.Load(san); f {
		return got.(string), nil
	}

	token, err := c.CreateServiceAccountToken(ns, sa)
	if err != nil {
		return "", err
	}
	cachedTokens.Store(san, token)
	return token, nil
}

func GenerateKey(ns, sa string) (KeyPair, error) {
	san := san(ns, sa)

	if got, f := cachedKeys.Load(san); f {
		return got.(KeyPair), nil
	}

	options := pkiutil.CertOptions{
		Host:       san,
		RSAKeySize: 2048,
	}
	// Generate the cert/key, send CSR to CA.
	csrPEM, keyPEM, err := pkiutil.GenCSR(options)
	if err != nil {
		return KeyPair{}, err
	}
	kp := KeyPair{KeyPEM: keyPEM, CsrPEM: csrPEM}
	cachedKeys.Store(san, kp)
	return kp, nil
}
