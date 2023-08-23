package config

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/howardjohn/pilot-load/pkg/kube"
	"github.com/howardjohn/pilot-load/pkg/simulation/model"
)

type SecretSpec struct {
	Namespace string
	Name      string
}

type Secret struct {
	Spec  *SecretSpec
	index int
}

var (
	_ model.Simulation            = &Secret{}
	_ model.RefreshableSimulation = &Secret{}
)

func NewSecret(s SecretSpec) *Secret {
	return &Secret{Spec: &s}
}

func (s *Secret) Run(ctx model.Context) (err error) {
	return kube.Apply(ctx.Client, s.getSecret())
}

func (s *Secret) Refresh(ctx model.Context) error {
	s.index = (s.index + 1) % len(crts)
	return s.Run(ctx)
}

func (s *Secret) Cleanup(ctx model.Context) error {
	return kube.Delete(ctx.Client, s.getSecret())
}

func (s *Secret) getSecret() *v1.Secret {
	p := s.Spec
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.Name,
			Namespace: p.Namespace,
		},
		Data: map[string][]byte{
			"tls.crt": crts[s.index],
			"tls.key": keys[s.index],
		},
	}
}

var (
	crts = [][]byte{
		[]byte(`-----BEGIN CERTIFICATE-----
MIIC3DCCAcQCAQAwDQYJKoZIhvcNAQELBQAwNTEVMBMGA1UECgwMZXhhbXBsZSBJ
bmMuMRwwGgYDVQQDDBMqLiR7SU5HUkVTU19ET01BSU59MB4XDTIwMDgxNTAwMTkw
NVoXDTIxMDgxNTAwMTkwNVowMzESMBAGA1UEAwwJKi4ubmlwLmlvMR0wGwYDVQQK
DBRleGFtcGxlIG9yZ2FuaXphdGlvbjCCASIwDQYJKoZIhvcNAQEBBQADggEPADCC
AQoCggEBALZKJ7L0yXbWX62cWDC/+P/aaGMAHVBMlZ9Y3FJZtELnckbwjEyNDjKb
qbK00wg3WCR2MVFy00F9C9fxwkp0owVK6vXUBF/lHv7XaDCFZB/6svxciZW7Xo7h
TutDNGnN8cZa8TmqfwFS47O8eJaZBaVrx09fziu60H66naJDIeNQhn5yfsQimLCH
7g99x2L0mekfeRcVUMhoE4v/83yhg6iJQU7/if1jnNC2QXelWv1OvFj1xOw4Cd1S
ahXNy2yPrMh8yWsQrd4FCXZtGZEctfNCmH2Q7mmsQuwpVccwKpfQLLvsirCL6eOV
T5H9XLN5jxSD+m6krkrbhfExuyrBCzMCAwEAATANBgkqhkiG9w0BAQsFAAOCAQEA
1MkYZ+AfeE2MVgFMZWgH0oP7U5BOs4GjwGdkPgEJLAqfgA1j6cmG771m9Mq8vESL
sPFU3jgCfMbja74vYsUgqIucUZWPMYySSxyt/mnV/62QheKmuEBq79s/Nqyx1gyV
hNwhuRs56WVSLLlHht0KxfbW3ZLbEw4LoCz4fFEmXHmzaaX/E/Qeqa/RgkAG5QPs
iZOsqbj/GWwOg2s08ahiuNyycffo+bZoj9ZTfvKfOvhMm94s8jIjcSRBN4KlFyzr
ONByLGB4V1rlkMFWqVGCj7t3Zbdub7h8e7w/vPEGUR6Y1PXMDvqm5SuAhJ3ZRn0C
eOw2HZSq0ZeOYXFhVEpsWA==
-----END CERTIFICATE-----
`),
		[]byte(`
-----BEGIN CERTIFICATE-----
MIIC3DCCAcQCAQAwDQYJKoZIhvcNAQELBQAwNTEVMBMGA1UECgwMZXhhbXBsZSBJ
bmMuMRwwGgYDVQQDDBMqLiR7SU5HUkVTU19ET01BSU59MB4XDTIwMDgxNTAwMjQ0
MVoXDTIxMDgxNTAwMjQ0MVowMzESMBAGA1UEAwwJKi4ubmlwLmlvMR0wGwYDVQQK
DBRleGFtcGxlIG9yZ2FuaXphdGlvbjCCASIwDQYJKoZIhvcNAQEBBQADggEPADCC
AQoCggEBAL/Okp8ma34hvC+bdekgjo8xkeL8mJYPhmXYUdk+ICyVoEW5g85pK8Q8
xL4pTPJl4KsXss+n/NKXAybOifyGD6BZtq+JEE4BoCpbVYPUl+6299arah9tpfCJ
qWwOHEop5x994XKJsrn9pF4kG7hIC3mp7wFSUDnufsO/8EECTAgHi4MKmaO9TqMw
k4aJ8q3cSzdjVxhXJCaprFAtB9QH3DJ9iiGzMqSJlCaV5HqVxjHVcmRRs6+Va4re
X4pQAFmUvUtDpqVtVgzocm07p5yhNAgAL/IsA4580N2KgH+hws9cMDU7nO5nKD7V
dB4p6YbO+r6gpuoGolB3/menvpGP380CAwEAATANBgkqhkiG9w0BAQsFAAOCAQEA
CofwHfYcADcHJ9lo7/Ki6wOckHhdYCnCBWLlAumZbdTfvOqytriYUb91e8za5qgE
TCYhUJW1W+Y+Rn7h2KPC5nLbAb2CJHF6W7tu7lPtCps+xQvShB4WztNbi/oiMlPF
gOOvbANqhGqU4BLyECeCniGhphNqZMTWojBZ41D1FMKcZTXm6vS77sFvqHqlI6Vi
rW1IL0crkO0ekBMqs/0q+1fRsnXaOPeFsqVWlvrGtroA6yoZfDmsw75rM7Ij2EGZ
sSZ77S9SlD5xYeKLisC+12jr/nuluQjWt9ZBEsoqLIjGTkyKKtKxHHkYZouDuxSr
9Knh2XBb/WrQnNgU7gq2Bg==
-----END CERTIFICATE-----
`),
	}
	keys = [][]byte{
		[]byte(`-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC2Siey9Ml21l+t
nFgwv/j/2mhjAB1QTJWfWNxSWbRC53JG8IxMjQ4ym6mytNMIN1gkdjFRctNBfQvX
8cJKdKMFSur11ARf5R7+12gwhWQf+rL8XImVu16O4U7rQzRpzfHGWvE5qn8BUuOz
vHiWmQWla8dPX84rutB+up2iQyHjUIZ+cn7EIpiwh+4Pfcdi9JnpH3kXFVDIaBOL
//N8oYOoiUFO/4n9Y5zQtkF3pVr9TrxY9cTsOAndUmoVzctsj6zIfMlrEK3eBQl2
bRmRHLXzQph9kO5prELsKVXHMCqX0Cy77Iqwi+njlU+R/VyzeY8Ug/pupK5K24Xx
MbsqwQszAgMBAAECggEALlV29cPGmZAvzZ8Zw3poPhAzzEYxLUHqHhCmH8BxUzgl
Eeb+ok6QN0jdv3C62zHGE65/Jqa7D8BLDF6E9gvji+rZAhcb7Yv9buUttHeTVZWR
fRfAWPDBPiwCtUXlwqb4N2TSP8gYdCuvePYE6LKIft2AUaqWawMXD787Zg0ORgq5
8i6ZE1b0VIFR/z+Wn7Yhe8D86/P/oin98jBQZ1vLx/0yV4zUnYlqeo9Z3JkahQTV
DdWlsKNas7OazWdi1CJS+oF8wvgNxTAJWHvZBtYpIUqhV5g5aTvlBvWE7ZkN6bwb
bFa2dyMLbRV36gYvGBrWjJ4EUUKlrGzVK7fjX7A+kQKBgQDaC5uYbRQf6n3i215C
WagcEEG5uC9MaHcZODBh6eeH8g8OZJs05r64wSEmvo/mG8REALCH8zpAH71ECoFW
pksRxwC8iFr+NpRBng/ec10ztd9l8fW0Yhz7N+7rkiyD+AN0Hn3VuF2aFaf0Hjb6
V5TWmHm2OOfz+o6vn3Y4eqTO3QKBgQDWBToBBRVM3DSLPbA6BULCum4qYQFRJpf+
18Gi2g/H1XG17n4WKo9/ldb+8D11J9/yHEs1UKJjrk1Xc7yT00BohkmftFELkPm7
Y5xPBclYQ0rgI4KfV/nGuQO8vyw62gtIJSkT2o8XN60Kzw8uOe5kdfIX9hFz2a6l
MdfNMs45TwKBgA3nXZWbpwPd/QcBPAJ5GxonAznnf8SciLOn/JXRx3zIt6MQUUFP
UWwQjJ+e2SgwLxSzAo64uMcr/vKexN6UngbVLLvY6gx5yHxiqtphetj4SPWEN0m4
U+bFC0wkNwh3QSkfZKDDL9zKcrpDTvgpq4j/kgtHl6rcGEsknPI/B9FVAoGAMan0
09fCIZvX9ZfTFSOzYkyw09S+4X37N4AJxyijENRPFtDJIYuu0QSMZ4yINm+SYDSA
n1ae2FLST8DjucoD4D2JSC4nwG9cBEgRNaU6G+lBrtGOtjtMEvlMDLiwItgGVi+J
YLoPCmw5E5EJDMkUsOtNypGnayLQjDUMxulLQbECgYEAlqVBtvvqKnx7grUNTNT8
JKlvGEO9IiBjt1WAtyTO3uPDJJpiP9RULiab1pLAEI102yzYRBb2lwwKEuWboIEV
tycfBVQBBVBrrzv1FVEcjtVqudBq3RiIYd5o3XjCXGASTpKcT8wECsCXB57wvFsg
HqysUoAFvv3IxkBLdeUpbJk=
-----END PRIVATE KEY-----
`),
		[]byte(`-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC/zpKfJmt+Ibwv
m3XpII6PMZHi/JiWD4Zl2FHZPiAslaBFuYPOaSvEPMS+KUzyZeCrF7LPp/zSlwMm
zon8hg+gWbaviRBOAaAqW1WD1JfutvfWq2ofbaXwialsDhxKKecffeFyibK5/aRe
JBu4SAt5qe8BUlA57n7Dv/BBAkwIB4uDCpmjvU6jMJOGifKt3Es3Y1cYVyQmqaxQ
LQfUB9wyfYohszKkiZQmleR6lcYx1XJkUbOvlWuK3l+KUABZlL1LQ6albVYM6HJt
O6ecoTQIAC/yLAOOfNDdioB/ocLPXDA1O5zuZyg+1XQeKemGzvq+oKbqBqJQd/5n
p76Rj9/NAgMBAAECggEAe1P8rL5MYZaZZNcF0rcvUt0hm5ylE9+5n+SehBvRHjm7
CvoEyQSQsqYMLuVpZ3agJgKf46t1AYc97Ibi7G7av1TQBUARLscW0AVYD+LzKfeV
lf8zxi9/ghFR0XulLv6QcIxFGJt3QuXW+P0ooa4ZSso8NlJR6V3zPjQ869/pOUNb
t9HJjjXhNATyYzqt9zDNLu8rkqz7yruuqoga/4tUZCO602i9Vh10FmFFI6g7piov
Y0bHLPfGqDrTLhVvOW/48D9Ls+iEw9NiFOEbZzFRk4fSSTfEjG8omwh84wgz5fmL
2//7hI+Ylz9bcytZ4ceffmIjwWfKHnWm6ULZ6EYSVQKBgQDd310W8v5Cx7grUCSE
nsyiQMryrKjyHgSAfgk08oWWpEU0+uBsFC3weiBp7xjh1UHSKq+PifVVc07b1VWu
aF3GXkAnC6LCTvd9dHnBxWZjylYIgtV2SRyGr/hIZlKq8mOnfq/FuD3kKZKeU6cm
vwGGfdZIjvGPBWz812M7NjELSwKBgQDdT1JNYZKPyqQtCiUAgCzr8XWreGFS7q1d
NdvJphkhOTEXXeD7GLD4LHdBa7ypLN6h1puSPGnX/fN0ONQpCgY7z3gILK/Jqzd+
77Xce7QXu+QvUGpT66d/AyRSCud+kXqBpPWvPvzX+h4hiecofuEyVn478vH2x/Fq
V4Vxbzx6RwKBgQDFUH5uCV27r/gGdPh1BPCBn1Oda5W39KAWUYAImWHabW6qxi3N
kEimo0WuUBd1x30I1jNZWNxYyPopoNjZCTHUVz+AOeXeHfIVnP8nJ1F+j5Phb9E8
p5p54YbRhEYihvu/GnhhQw+vmJUuvsBZQeauX7ywvIbwpWeemEJEh1YobwKBgQC1
0hxpDLfPwQmfI02BGs1NT0SAitdSvlraUIxxIDBXNliZvPxA72k9i7KyoeQPDZkf
V2TbAR1oYfCpVKMh0GWMsAgKl0QZKLzgYeqE6XDtauWu5Z9lsR8cX6VwbhsAxl7i
sndS8ini+0/T+Ctc/tjfdWYitJeMS3qRBrTQnDYQswKBgHhPEiKsVv1y/cCxnKWi
neXJ9+wuPxUmk6FiCZnkmJIrQ7T7DWAMPLjaoIkz7IpCgo4cPJTet8ZonJJ7S2O0
nuB+9FLxOkDRlmx/aI1rFuoSp4Bn2rdXWFlqnd3aZWmGHQDizjTySVnEB+Sp3g8b
leoV32nyeTd7AnQ+TrQbSPHK
-----END PRIVATE KEY-----
`),
	}
)
