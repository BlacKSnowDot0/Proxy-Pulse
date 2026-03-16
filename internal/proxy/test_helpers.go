package proxy

import (
	"net"
	"strconv"
)

func netSplitHostPort(address string) (string, int, error) {
	host, portRaw, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portRaw)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}
