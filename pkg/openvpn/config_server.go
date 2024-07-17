// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package openvpn

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path"

	"github.com/gardener/vpn2/pkg/network"
)

var (
	//go:embed assets/server-config.template
	seedServerConfigTemplate string
	//go:embed assets/server-for-client-config.template
	configFromServerForClientTemplate string
	//go:embed assets/server-for-client-ha-config.template
	configFromServerForClientHATemplate string
)

type SeedServerValues struct {
	Device             string
	IPFamilies         string
	StatusPath         string
	OpenVPNNetwork     network.CIDR
	OpenVPNNetworkPool network.CIDR
	ShootNetworks      []network.CIDR
	HAVPNClients       int
	IsHA               bool
	VPNIndex           int
	LocalNodeIP        string
}

func generateSeedServerConfig(cfg SeedServerValues) (string, error) {
	buf := &bytes.Buffer{}
	if err := executeTemplate("openvpn.cfg", buf, seedServerConfigTemplate, &cfg); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// generateConfigForClientFromServer generates the config that the server sends to non HA shoot vpn clients
func generateConfigForClientFromServer(cfg SeedServerValues) (string, error) {
	buf := &bytes.Buffer{}
	if err := executeTemplate("vpn-shoot-client", buf, configFromServerForClientTemplate, &cfg); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// generateConfigForClientHAFromServer generates the config that the server sends to HA shoot vpn clients
func generateConfigForClientHAFromServer(cfg SeedServerValues, startIP string) (string, error) {
	buf := &bytes.Buffer{}
	data := map[string]any{"OpenVPNNetwork": cfg.OpenVPNNetwork, "StartIP": startIP}
	if err := executeTemplate("vpn-shoot-client-ha", buf, configFromServerForClientHATemplate, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

const (
	openvpnClientConfigDir    = "/client-config-dir"
	openvpnClientConfigPrefix = "vpn-shoot-client"
)

func WriteServerConfigFiles(v SeedServerValues) error {
	openvpnConfig, err := generateSeedServerConfig(v)
	if err != nil {
		return fmt.Errorf("error %w: Could not generate openvpn config from %v", err, v)
	}
	if err := os.WriteFile(defaultOpenVPNConfigFile, []byte(openvpnConfig), 0o644); err != nil {
		return err
	}

	vpnShootClientConfig, err := generateConfigForClientFromServer(v)
	if err != nil {
		return fmt.Errorf("error %w: Could not generate shoot client config from %v", err, v)
	}
	err = os.Mkdir(openvpnClientConfigDir, 0750)
	if err != nil && !os.IsExist(err) {
		return err
	}
	if err := os.WriteFile(path.Join(openvpnClientConfigDir, openvpnClientConfigPrefix), []byte(vpnShootClientConfig), 0o644); err != nil {
		return err
	}

	if v.IsHA {
		for i := 0; i < v.HAVPNClients; i++ {
			startIP := v.OpenVPNNetwork.IP
			startIP[len(startIP)-1] = byte(v.VPNIndex*64 + i + 2)
			vpnShootClientConfigHA, err := generateConfigForClientHAFromServer(v, startIP.String())
			if err != nil {
				return fmt.Errorf("error %w: Could not generate ha shoot client config %d from %v", err, i, v)
			}
			if err := os.WriteFile(fmt.Sprintf("%s-%d", path.Join(openvpnClientConfigDir, openvpnClientConfigPrefix), i), []byte(vpnShootClientConfigHA), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}
