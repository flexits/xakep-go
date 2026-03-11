package main

import (
	"net"
	"os/exec"
	"strings"
)

func retrieveArpTable() (map[string]string, bool) {
	result := make(map[string]string)

	data, err := exec.Command("arp", "-a").Output()
	if err != nil {
		return result, false
	}

	for l := range strings.Lines(string(data)) {
		l = strings.TrimSpace(l)
		if len(l) < 24 {
			continue
		}
		tokens := strings.Fields(l)
		if len(tokens) != 3 {
			continue
		}
		ipStr := tokens[0]
		macStr := tokens[1]
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		mac, err := net.ParseMAC(macStr)
		if err != nil {
			continue
		}
		if isUnicastMac(mac) {
			result[ipStr] = macStr
		}
	}

	return result, true
}
