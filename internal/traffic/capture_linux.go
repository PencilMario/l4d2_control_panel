//go:build linux

package traffic

import (
	"context"
	"errors"
	"net"
	"time"

	"golang.org/x/sys/unix"
)

const ethernetProtocolAll = 0x0003

func StartCapture(ctx context.Context, observer Observer) (<-chan error, error) {
	interfaces, err := captureInterfaces()
	if err != nil {
		return nil, err
	}
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, int(htons(ethernetProtocolAll)))
	if err != nil {
		return nil, err
	}
	errorsCh := make(chan error, 1)
	go captureLoop(ctx, fd, interfaces, observer, errorsCh)
	return errorsCh, nil
}

func captureLoop(ctx context.Context, fd int, interfaces map[int]struct{}, observer Observer, errorsCh chan<- error) {
	defer close(errorsCh)
	defer unix.Close(fd)
	buffer := make([]byte, 256)
	refresh := time.NewTicker(5 * time.Second)
	defer refresh.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-refresh.C:
			if updated, err := captureInterfaces(); err == nil {
				interfaces = updated
			}
		default:
		}

		n, source, err := unix.Recvfrom(fd, buffer, unix.MSG_DONTWAIT|unix.MSG_TRUNC)
		if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
			poll := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			_, _ = unix.Poll(poll, 250)
			continue
		}
		if err != nil {
			errorsCh <- err
			return
		}
		packetSource, ok := source.(*unix.SockaddrLinklayer)
		if !ok {
			continue
		}
		if _, ok := interfaces[packetSource.Ifindex]; !ok {
			continue
		}
		headerLength := n
		if headerLength > len(buffer) {
			headerLength = len(buffer)
		}
		if packet, ok := decodeFrame(buffer[:headerLength], uint64(n)); ok {
			observer.Observe(packet)
		}
	}
}

func captureInterfaces() (map[int]struct{}, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	active := make(map[int]struct{})
	for _, iface := range all {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			active[iface.Index] = struct{}{}
		}
	}
	return active, nil
}

func htons(value uint16) uint16 {
	return value<<8 | value>>8
}
