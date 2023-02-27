package model

type IstioRootNSConfig struct {
	Default     *bool              `json:"default,omitempty"`
	EnvoyFilter *EnvoyFilterConfig `json:"envoyFilter,omitempty"`
	Sidecar     *SidecarConfig     `json:"sidecar,omitempty"`
}

type IstioNSConfig struct {
	Default     *bool              `json:"default,omitempty"`
	EnvoyFilter *EnvoyFilterConfig `json:"envoyFilter,omitempty"`
	Sidecar     *SidecarConfig     `json:"sidecar,omitempty"`
}

type IstioApplicationConfig struct {
	Default         *bool                  `json:"default,omitempty"`
	DestinationRule *DestinationRuleConfig `json:"destinationRule,omitempty"`
	EnvoyFilter     *EnvoyFilterConfig     `json:"envoyFilter,omitempty"`
	Sidecar         *SidecarConfig         `json:"sidecar,omitempty"`
	VirtualService  *VirtualServiceConfig  `json:"virtualService,omitempty"`
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
