//go:build !windows

package worker

import (
	"fmt"
	"os"
	"syscall"
)

// AcquireDatabaseLock prevents two web processes from recovering or executing
// each other's work. The OS releases the lock even after an abrupt exit.
func AcquireDatabaseLock(dbPath string) (func(), error) {
	file, err := os.OpenFile(dbPath+".worker.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("another Aycorn web process is using this database: %w", err)
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}
