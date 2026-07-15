//go:build windows

package content

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func atomicReplaceFile(source, target string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	for attempt := 0; ; attempt++ {
		err = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil || attempt == 999 || (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED)) {
			return err
		}
		time.Sleep(time.Millisecond)
	}
}

func readAtomicFile(path string) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		raw, err := os.ReadFile(path)
		if err == nil || attempt == 999 || (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED)) {
			return raw, err
		}
		time.Sleep(time.Millisecond)
	}
}
