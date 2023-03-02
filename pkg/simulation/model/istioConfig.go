package model

type IstioRootNSConfig struct {
	Default               bool                         `json:"default,omitempty"`
	EnvoyFilter           *EnvoyFilterConfig           `json:"envoyFilter,omitempty"`
	Sidecar               *SidecarConfig               `json:"sidecar,omitempty"`
	Telemetry             *TelemetryConfig             `json:"telemetry,omitempty"`
	PeerAuthentication    *PeerAuthenticationConfig    `json:"peerAuthentication,omitempty"`
	RequestAuthentication *RequestAuthenticationConfig `json:"requestAuthentication,omitempty"`
	AuthorizationPolicy   *AuthorizationPolicyConfig   `json:"authorizationPolicy,omitempty"`
}

type IstioNSConfig struct {
	Default               bool                         `json:"default,omitempty"`
	EnvoyFilter           *EnvoyFilterConfig           `json:"envoyFilter,omitempty"`
	Sidecar               *SidecarConfig               `json:"sidecar,omitempty"`
	Telemetry             *TelemetryConfig             `json:"telemetry,omitempty"`
	PeerAuthentication    *PeerAuthenticationConfig    `json:"peerAuthentication,omitempty"`
	RequestAuthentication *RequestAuthenticationConfig `json:"requestAuthentication,omitempty"`
	AuthorizationPolicy   *AuthorizationPolicyConfig   `json:"authorizationPolicy,omitempty"`
}

type IstioApplicationConfig struct {
	Default               bool                         `json:"default,omitempty"`
	DestinationRule       *DestinationRuleConfig       `json:"destinationRule,omitempty"`
	EnvoyFilter           *EnvoyFilterConfig           `json:"envoyFilter,omitempty"`
	Sidecar               *SidecarConfig               `json:"sidecar,omitempty"`
	VirtualService        *VirtualServiceConfig        `json:"virtualService,omitempty"`
	Telemetry             *TelemetryConfig             `json:"telemetry,omitempty"`
	PeerAuthentication    *PeerAuthenticationConfig    `json:"peerAuthentication,omitempty"`
	RequestAuthentication *RequestAuthenticationConfig `json:"requestAuthentication,omitempty"`
	AuthorizationPolicy   *AuthorizationPolicyConfig   `json:"authorizationPolicy,omitempty"`
}

type DestinationRuleConfig struct {
	// Defaults to parent name. Setting allows a stable identifier
	Name string `json:"name,omitempty"`
}

type EnvoyFilterConfig struct {
	// Defaults to parent name. Setting allows a stable identifier
	Name string `json:"name,omitempty"`
}

type SidecarConfig struct {
	// Defaults to parent name. Setting allows a stable identifier
	Name string `json:"name,omitempty"`
}

type VirtualServiceConfig struct {
	// Defaults to parent name. Setting allows a stable identifier
	Name     string   `json:"name,omitempty"`
	Gateways []string `json:"gateways,omitempty"`
}

type RequestAuthenticationConfig struct {
	// Defaults to parent name. Setting allows a stable identifier
	Name string `json:"name,omitempty"`
}

type AuthorizationPolicyConfig struct {
	// Defaults to parent name. Setting allows a stable identifier
	Name string `json:"name,omitempty"`
}

type PeerAuthenticationConfig struct {
	// Defaults to parent name. Setting allows a stable identifier
	Name string `json:"name,omitempty"`
}

type TelemetryConfig struct {
	// Defaults to parent name. Setting allows a stable identifier
	Name string `json:"name,omitempty"`
}
