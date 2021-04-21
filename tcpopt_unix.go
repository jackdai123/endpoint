// +build linux freebsd dragonfly

package endpoint

import (
	"errors"
	"os"
	"syscall"
	"time"
)

func setKeepAlive(fd int, d time.Duration) error {
	if d <= 0 {
		return errors.New("invalid time duration")
	}
	secs := int(d / time.Second)
	if err := os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1)); err != nil {
		return err
	}
	if err := os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, secs)); err != nil {
		return err
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE, secs))
}
