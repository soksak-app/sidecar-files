//go:build darwin

package darwin

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/soksak-app/sidecar-files/src/platform"
)

type implementation struct{}

func init() { platform.Register(implementation{}) }

// oEvtOnly 는 감시만을 위해 여는 플래그(O_EVTONLY)다. 연 디렉터리가 있는 볼륨의 꺼내기를 막지 않는다.
const oEvtOnly = 0x8000

// Watch 는 디렉터리나 정규 파일의 기술자에 EVFILT_VNODE 를 걸고, 멈춤 신호를 받을 파이프를 같은 kqueue 에 건다.
// 디렉터리는 항목의 변화(NOTE_WRITE, NOTE_EXTEND)와 자신의 삭제와 이름 변경을, 정규 파일은 내용의 쓰기와 늘어남
// (NOTE_WRITE, NOTE_EXTEND)을 받는다.
func (implementation) Watch(path string, changed func(), failed func(error)) (func() error, error) {
	fd, err := syscall.Open(path, oEvtOnly, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return nil, errors.Join(fmt.Errorf("stat %s: %w", path, err), syscall.Close(fd))
	}
	var fflags uint32
	switch stat.Mode & syscall.S_IFMT {
	case syscall.S_IFDIR:
		fflags = syscall.NOTE_WRITE | syscall.NOTE_DELETE | syscall.NOTE_RENAME | syscall.NOTE_EXTEND
	case syscall.S_IFREG:
		fflags = syscall.NOTE_WRITE | syscall.NOTE_EXTEND
	default:
		return nil, errors.Join(fmt.Errorf("watch %s: not a regular file or directory", path), syscall.Close(fd))
	}
	kq, err := syscall.Kqueue()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("kqueue: %w", err), syscall.Close(fd))
	}
	var pipe [2]int
	if err := syscall.Pipe(pipe[:]); err != nil {
		return nil, errors.Join(fmt.Errorf("pipe: %w", err), syscall.Close(fd), syscall.Close(kq))
	}
	closeAll := func() error {
		var first error
		for _, each := range []int{fd, kq, pipe[0]} {
			if err := syscall.Close(each); err != nil && first == nil {
				first = fmt.Errorf("close watch of %s: %w", path, err)
			}
		}
		return first
	}
	var events [2]syscall.Kevent_t
	syscall.SetKevent(&events[0], fd, syscall.EVFILT_VNODE, syscall.EV_ADD|syscall.EV_CLEAR)
	events[0].Fflags = fflags
	syscall.SetKevent(&events[1], pipe[0], syscall.EVFILT_READ, syscall.EV_ADD)
	if _, err := syscall.Kevent(kq, events[:], nil, nil); err != nil {
		return nil, errors.Join(fmt.Errorf("kevent %s: %w", path, err), closeAll(), syscall.Close(pipe[1]))
	}
	go func() {
		defer func() {
			if err := closeAll(); err != nil {
				failed(err)
			}
		}()
		received := make([]syscall.Kevent_t, 4)
		for {
			n, err := syscall.Kevent(kq, nil, received, nil)
			if err == syscall.EINTR {
				continue
			}
			if err != nil {
				failed(fmt.Errorf("kevent %s: %w", path, err))
				return
			}
			for _, event := range received[:n] {
				if int(event.Ident) == pipe[0] {
					return
				}
			}
			changed()
		}
	}()
	// 쓰는 끝을 닫으면 읽는 끝에 EOF 가 생기고, 감시 고루틴이 그 이벤트를 받아 끝난다.
	return func() error { return syscall.Close(pipe[1]) }, nil
}
