//go:build !windows && !linux

package main

func retrieveArpTable() (map[string]string, bool) {
	return make(map[string]string), false
}
