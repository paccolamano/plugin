package osutil

import (
	"fmt"
	"os/exec"
	"strings"
)

func CompilePlugin(srcDir, outputPath string) error {
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", outputPath, ".")
	cmd.Dir = srcDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("compilation failed: %w\n%s", err, msg)
		}
		return fmt.Errorf("compilation failed: %w", err)
	}
	return nil
}
