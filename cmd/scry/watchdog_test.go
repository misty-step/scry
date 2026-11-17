package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRule_Reliability_WatchdogRestartsOnlyUnhealthyContainers(t *testing.T) {
	dir := t.TempDir()
	fake := "#!/bin/sh\ncase \"$1\" in\ninspect) echo \"$STATE\";;\nrestart) echo \"$*\" >> \"$RESTART_LOG\";;\n*) exit 1;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(fake), 0700); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"healthy", "starting", "stopped", "unhealthy", "unknown"} {
		t.Run(state, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "restarts")
			cmd := exec.Command("sh", "../../deploy/watchdog.sh", "scry-test")
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "STATE="+state, "RESTART_LOG="+log)
			out, err := cmd.CombinedOutput()
			if (err != nil) != (state == "unknown") {
				t.Fatalf("%s: %v", out, err)
			}
			b, _ := os.ReadFile(log)
			if state == "unhealthy" {
				if strings.TrimSpace(string(b)) != "restart --time 20 scry-test" {
					t.Fatalf("restart = %s", b)
				}
			} else if len(b) != 0 {
				t.Fatalf("unexpected restart for %s", state)
			}
		})
	}
}
