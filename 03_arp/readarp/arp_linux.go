package main

import (
	"bufio"
	"net"
	"os"
	"strings"
)

func retrieveArpTable() (map[string]string, bool) {
	result := make(map[string]string)

	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return result, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		l := scanner.Text()
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}

		tokens := strings.Fields(l)
		if len(tokens) < 4 {
			continue
		}

		ipStr := tokens[0]
		macStr := tokens[3]

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
