package utils

import "fmt"

const (
	MinAddrFormat = "000000000000000000000"
	MaxAddrFormat = "255.255.255.255:99999"
)

func GetAddrFormat(addr string) string {
	return fmt.Sprintf("%021s", addr)
}

func GetAddrNextFormat(addr string) string {
	return fmt.Sprintf("%s%c", addr[:len(addr)-1], addr[len(addr)-1]+1)
}
