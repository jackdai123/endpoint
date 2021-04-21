// +build linux freebsd dragonfly darwin

package endpoint

import (
	"os"
	"syscall"
)

func setNoDelay(fd int, noDelay TCPSocketOpt) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, int(noDelay)))
}
