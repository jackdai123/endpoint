// +build linux freebsd dragonfly

package endpoint

import (
	"os"
	"syscall"
)

func sysSocket(family, sotype, proto int) (int, error) {
	s, err := syscall.Socket(family, sotype|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, proto)

	// On Linux the SOCK_NONBLOCK and SOCK_CLOEXEC flags were
	// introduced in 2.6.27 kernel and on FreeBSD both flags were
	// introduced in 10 kernel. If we get an EINVAL error on Linux
	// or EPROTONOSUPPORT error on FreeBSD, fall back to using
	// socket without them.
	switch err {
	case nil:
		return s, nil
	default:
		return -1, os.NewSyscallError("socket", err)
	case syscall.EPROTONOSUPPORT, syscall.EINVAL:
	}

	// See ../syscall/exec_unix.go for description of ForkLock.
	syscall.ForkLock.RLock()
	s, err = syscall.Socket(family, sotype, proto)
	if err == nil {
		syscall.CloseOnExec(s)
	}
	syscall.ForkLock.RUnlock()
	if err != nil {
		return -1, os.NewSyscallError("socket", err)
	}

	if err = syscall.SetNonblock(s, true); err != nil {
		syscall.Close(s)
		return -1, os.NewSyscallError("setnonblock", err)
	}
	return s, nil
}
