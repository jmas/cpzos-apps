// Package backlight sets the panel backlight through the cpz-ui helper.
//
// The helper writes the m5ioe1 PWM registers over I2C; it is installed as
// /usr/lib/cpz/backlight and whitelisted in the cpz-ui sudoers rule. The
// session never calls it directly as root.
package backlight

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Helper is the installed backlight binary, owned by cpz-ui.
const Helper = "/usr/lib/cpz/backlight"

// Set applies a percentage between 0 and 100. A test can point CPZ_BACKLIGHT
// at a fake script; then the command runs directly instead of through sudo.
func Set(percent int) error {
	if percent < 0 || percent > 100 {
		return fmt.Errorf("brightness out of range: %d", percent)
	}
	path := os.Getenv("CPZ_BACKLIGHT")
	direct := path != ""
	if !direct {
		path = Helper
	}
	args := []string{"set", strconv.Itoa(percent)}

	var cmd *exec.Cmd
	if direct || os.Geteuid() == 0 {
		cmd = exec.Command(path, args...)
	} else {
		cmd = exec.Command("sudo", append([]string{"-n", path}, args...)...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}
