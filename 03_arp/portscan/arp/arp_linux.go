package arp

import (
	"bufio"
	"net"
	"os"
	"strings"
)

// retrieveArpTable возвращает набор соответствий
// сетевых и аппаратных адресов из кеша ARP операционной
// системы.
// Не-unicast адреса в коллекцию не включаются.
// В случае ошибки, возвращённая коллекция будет пустой.
func retrieveArpTable() map[string]string {
	result := make(map[string]string)

	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return result
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

	return result
}
