package security

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
	"time"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	pb "istio.io/api/security/v1alpha1"
)

// map of SAN to jwt token. Used to avoid repetitive calls
var cachedTokens sync.Map

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
		if t.expiration.After(time.Now().Add(time.Minute)) {
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

// NewCitadelClient create a CA client for Citadel.
func newCitadelClient(endpoint string, rootCert []byte) (pb.IstioCertificateServiceClient, error) {
	certPool := x509.NewCertPool()
	ok := certPool.AppendCertsFromPEM(rootCert)
	if !ok {
		return nil, fmt.Errorf("failed to append certificates")
	}
	config := tls.Config{
		RootCAs:            certPool,
		InsecureSkipVerify: true,
	}
	transportCreds := credentials.NewTLS(&config)

	conn, err := grpc.Dial(endpoint, grpc.WithTransportCredentials(transportCreds))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to endpoint %s", endpoint)
	}

	client := pb.NewIstioCertificateServiceClient(conn)
	return client, nil
}
