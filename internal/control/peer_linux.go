//go:build linux

package control

import (
	"errors"
	"net"
	"os"
	"syscall"
)

func SameUser(connection net.Conn) error {
	unix, ok := connection.(*net.UnixConn)
	if !ok {
		return errors.New("control connection is not Unix-domain")
	}
	raw, err := unix.SyscallConn()
	if err != nil {
		return err
	}
	var credentials *syscall.Ucred
	var socketErr error
	err = raw.Control(func(fd uintptr) {
		credentials, socketErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		return err
	}
	if socketErr != nil {
		return socketErr
	}
	if credentials == nil || int(credentials.Uid) != os.Getuid() {
		return errors.New("control peer uid mismatch")
	}
	return nil
}
