package admin

import (
	"net"
	"strings"
)

// hostAllowed accepts only loopback hosts. The listener already binds
// loopback; this additionally rejects DNS-rebinding requests whose Host
// header points elsewhere.
func hostAllowed(hostport string) bool {
	h := hostport
	if hp, _, err := net.SplitHostPort(hostport); err == nil {
		h = hp
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
