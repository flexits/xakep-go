package arp

import "net"

type arpReader struct {
	cache map[string]string
}

// NewArpReader возвращает новый инициализированный
// экземпляр читателя кеша ARP операционной системы.
func NewArpReader() *arpReader {
	return &arpReader{
		cache: retrieveArpTable(),
	}
}

// GetMac возвращает строковое представление аппаратного адреса из
// кеша ARP и true, ели значение найдено в кеше. В противном случае,
// возвращается пустая строка и false.
func (a *arpReader) GetMac(host string) (string, bool) {
	val, found := a.cache[host]
	return val, found
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
