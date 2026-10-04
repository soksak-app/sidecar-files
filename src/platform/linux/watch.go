//go:build linux

package linux

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/soksak-app/sidecar-files/src/platform"
)

type implementation struct{}

func init() { platform.Register(implementation{}) }

// watched 는 macOS 의 감시와 같은 변화다. 디렉터리 안의 항목이 생기거나 지워지거나 이름이 바뀌는 것과 디렉터리
// 자신이 지워지거나 옮겨지는 것이다.
const watched = syscall.IN_CREATE | syscall.IN_DELETE | syscall.IN_MOVED_FROM | syscall.IN_MOVED_TO |
	syscall.IN_DELETE_SELF | syscall.IN_MOVE_SELF | syscall.IN_ONLYDIR

// Watch 는 디렉터리에 inotify 감시를 걸고, 멈춤 신호를 받을 파이프와 함께 epoll 로 기다린다.
func (implementation) Watch(dir string, changed func(), failed func(error)) (func() error, error) {
	notify, err := syscall.InotifyInit1(syscall.IN_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("inotify: %w", err)
	}
	if _, err := syscall.InotifyAddWatch(notify, dir, watched); err != nil {
		return nil, errors.Join(fmt.Errorf("watch %s: %w", dir, err), syscall.Close(notify))
	}
	var pipe [2]int
	if err := syscall.Pipe2(pipe[:], syscall.O_CLOEXEC); err != nil {
		return nil, errors.Join(fmt.Errorf("pipe: %w", err), syscall.Close(notify))
	}
	poll, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("epoll: %w", err), syscall.Close(notify), syscall.Close(pipe[0]), syscall.Close(pipe[1]))
	}
	closeAll := func() error {
		var first error
		for _, each := range []int{notify, poll, pipe[0]} {
			if err := syscall.Close(each); err != nil && first == nil {
				first = fmt.Errorf("close watch of %s: %w", dir, err)
			}
		}
		return first
	}
	for _, fd := range []int{notify, pipe[0]} {
		event := syscall.EpollEvent{Events: syscall.EPOLLIN, Fd: int32(fd)}
		if err := syscall.EpollCtl(poll, syscall.EPOLL_CTL_ADD, fd, &event); err != nil {
			return nil, errors.Join(fmt.Errorf("epoll %s: %w", dir, err), closeAll(), syscall.Close(pipe[1]))
		}
	}
	go func() {
		defer func() {
			if err := closeAll(); err != nil {
				failed(err)
			}
		}()
		received := make([]syscall.EpollEvent, 2)
		buffer := make([]byte, 64*(syscall.SizeofInotifyEvent+syscall.NAME_MAX+1))
		for {
			n, err := syscall.EpollWait(poll, received, -1)
			if err == syscall.EINTR {
				continue
			}
			if err != nil {
				failed(fmt.Errorf("epoll %s: %w", dir, err))
				return
			}
			for _, event := range received[:n] {
				if int(event.Fd) == pipe[0] {
					return
				}
			}
			// 받은 변화를 모두 읽어 비운다. 변화의 내용은 쓰지 않고 디렉터리가 바뀌었다는 것만 알린다.
			if _, err := syscall.Read(notify, buffer); err != nil && err != syscall.EINTR {
				failed(fmt.Errorf("read watch of %s: %w", dir, err))
				return
			}
			changed()
		}
	}()
	// 쓰는 끝을 닫으면 읽는 끝에 EOF 가 생기고, 감시 고루틴이 그 이벤트를 받아 끝난다.
	return func() error { return syscall.Close(pipe[1]) }, nil
}
