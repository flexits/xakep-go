package main

import (
	"log"
	"net"

	"github.com/flexits/vt-enable"
	"github.com/goforj/godump"
)

func main() {
	vt.Enable()
	if arpResuls, ok := retrieveArpTable(); ok {
		godump.Dump(arpResuls)
	} else {
		log.Fatal("Unsupported OS!")
	}
}

// isUnicastMac возвращает true, если переданный адрес unicast.
func isUnicastMac(mac net.HardwareAddr) bool {
	// Если последний бит первого байта адреса равен 0, то адрес unicast.
	//
	// Используем битовое сравнение:
	//   xxxxxxx0
	// & 00000001
	//   --------
	//   00000000 == 0
	// (если последний бит == 0, получим 0 независимо от значения других бит);
	//
	//   xxxxxxx1
	// & 00000001
	//   --------
	//   00000001 == 1
	// (если последний бит == 1, получим 1 независимо от значения других бит).
	//
	return (mac[0] & 1) == 0
}
