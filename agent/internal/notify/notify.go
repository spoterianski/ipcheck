// Package notify shows desktop notifications.
package notify

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
)

// Send shows a desktop notification; on unsupported platforms it only logs.
func Send(title, body string, alarm bool) {
	log.Printf("notify: %s — %s", title, body)
	if runtime.GOOS != "darwin" {
		return
	}
	sound := "Glass"
	if alarm {
		sound = "Basso"
	}
	script := fmt.Sprintf("display notification %s with title %s sound name %q", quote(body), quote(title), sound)
	if err := exec.Command("osascript", "-e", script).Run(); err != nil {
		log.Printf("notify: osascript: %v", err)
	}
}

func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
