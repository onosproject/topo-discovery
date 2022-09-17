// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package types

type OpenconfigInterfacesInterfacesInterface struct {
	OpenconfigInterfacesInterface []struct {
		Name                         string `json:"name"`
		OpenconfigIfEthernetEthernet struct {
			State struct {
				GooglePinsInterfacesTransceiverQualified bool `json:"google-pins-interfaces:transceiver-qualified"`
			} `json:"state"`
		} `json:"openconfig-if-ethernet:ethernet"`
		State struct {
			CPU        bool   `json:"cpu"`
			Management bool   `json:"management"`
			Name       string `json:"name"`
			Type       string `json:"type"`
		} `json:"state"`
		Subinterfaces struct {
			Subinterface []struct {
				Index              int `json:"index"`
				OpenconfigIfIPIpv4 struct {
					Addresses struct {
						Address []struct {
							IP string `json:"ip"`
						} `json:"address"`
					} `json:"addresses"`
					Neighbors struct {
						Neighbor []struct {
							IP    string `json:"ip"`
							State struct {
								IP               string `json:"ip"`
								LinkLayerAddress string `json:"link-layer-address"`
							} `json:"state"`
						} `json:"neighbor"`
					} `json:"neighbors"`
					State struct {
						Enabled bool `json:"enabled"`
						Mtu     int  `json:"mtu"`
					} `json:"state"`
				} `json:"openconfig-if-ip:ipv4"`
				OpenconfigIfIPIpv6 struct {
					State struct {
						Enabled bool `json:"enabled"`
					} `json:"state"`
				} `json:"openconfig-if-ip:ipv6"`
				State struct {
					Index int `json:"index"`
				} `json:"state"`
			} `json:"subinterface"`
		} `json:"subinterfaces"`
	} `json:"openconfig-interfaces:interface"`
}
