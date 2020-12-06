package rm

import (
	"context"
	"time"
	"os/exec"
	"io"
	"fmt"	
)

func Command(toStdin string, timeout time.Duration, exe string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := io.WriteString(stdin, toStdin); err != nil {
		return err
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		return err
	}
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command timed out (> %s): %s", timeout, cmd)
	}
	return nil
}