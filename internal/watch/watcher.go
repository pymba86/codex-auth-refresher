package watch

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type Watcher struct {
	fd      int
	events  chan struct{}
	errors  chan error
	done    chan struct{}
	closeMu sync.Once
	wg      sync.WaitGroup
}

func New(dir string) (*Watcher, error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	mask := uint32(syscall.IN_CREATE | syscall.IN_CLOSE_WRITE | syscall.IN_MOVED_TO | syscall.IN_DELETE | syscall.IN_ATTRIB | syscall.IN_DELETE_SELF | syscall.IN_MOVE_SELF)
	if _, err := syscall.InotifyAddWatch(fd, dir, mask); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	w := &Watcher{
		fd:     fd,
		events: make(chan struct{}, 1),
		errors: make(chan error, 1),
		done:   make(chan struct{}),
	}
	w.wg.Add(1)
	go w.readLoop()
	return w, nil
}

func (w *Watcher) Events() <-chan struct{} {
	return w.events
}

func (w *Watcher) Errors() <-chan error {
	return w.errors
}

func (w *Watcher) Close() error {
	var closeErr error
	w.closeMu.Do(func() {
		close(w.done)
		closeErr = syscall.Close(w.fd)
		w.wg.Wait()
	})
	if errors.Is(closeErr, syscall.EBADF) {
		return nil
	}
	return closeErr
}

func (w *Watcher) readLoop() {
	defer w.wg.Done()
	defer close(w.events)
	defer close(w.errors)

	buf := make([]byte, 4096)
	for {
		n, err := syscall.Read(w.fd, buf)
		if err != nil {
			select {
			case <-w.done:
				return
			default:
			}
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				time.Sleep(25 * time.Millisecond)
				continue
			}
			select {
			case w.errors <- err:
			default:
			}
			return
		}
		if n < syscall.SizeofInotifyEvent {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		offset := 0
		for offset+syscall.SizeofInotifyEvent <= n {
			event := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[offset]))
			nameStart := offset + syscall.SizeofInotifyEvent
			nameEnd := nameStart + int(event.Len)
			if nameEnd > n {
				break
			}
			name := strings.TrimRight(string(buf[nameStart:nameEnd]), "\x00")
			if shouldEmit(event.Mask, name) {
				select {
				case w.events <- struct{}{}:
				default:
				}
			}
			offset = nameEnd
		}
	}
}

func shouldEmit(mask uint32, name string) bool {
	if name != "" && !strings.EqualFold(filepath.Ext(name), ".json") {
		return false
	}
	interesting := uint32(syscall.IN_CREATE | syscall.IN_CLOSE_WRITE | syscall.IN_MOVED_TO | syscall.IN_DELETE | syscall.IN_ATTRIB | syscall.IN_DELETE_SELF | syscall.IN_MOVE_SELF)
	return mask&interesting != 0
}
