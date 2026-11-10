//go:build darwin

package monitor

import (
	"os"
	"testing"
)

func TestReadPSCurrentProcess(t *testing.T) {
	procs, err := ReadPSErr()
	if err != nil {
		t.Fatal(err)
	}
	pid := int32(os.Getpid())
	var found *Proc
	for i := range procs {
		if procs[i].PID == pid {
			found = &procs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("pid %d not in ps table (%d rows) stderr=%s", pid, len(procs), LastPSStderr())
	}
	if found.RSS == 0 {
		t.Fatal("rss was 0")
	}
}
