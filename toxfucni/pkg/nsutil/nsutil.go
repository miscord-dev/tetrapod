package nsutil

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/vishvananda/netns"
)

func init() {
	nsName := os.Getenv("NSUTIL_CREATED_NAMESPACE_NAME")

	if nsName == "" {
		return
	}

	_, err := netns.NewNamed(nsName)

	if err != nil {
		panic(err)
	}

	os.Exit(0)
}

func CreateNamespace(name string) (netns.NsHandle, error) {
	cmd := exec.Command("/proc/self/exe")
	cmd.Env = append(os.Environ(), "NSUTIL_CREATED_NAMESPACE_NAME="+name)

	b, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to create netns namespace %s: %w", string(b), err)
	}

	return netns.GetFromName(name)
}

func RunInNamespace(handle netns.NsHandle, fn func() error) (err error) {
	runtime.LockOSThread()
	defer func() {
		if err == nil {
			runtime.UnlockOSThread()
		}
	}()

	cur, err := netns.Get()
	if err != nil {
		return err
	}
	defer func() {
		if e := netns.Set(cur); e != nil {
			err = fmt.Errorf("failed to recover netns: %w", err)
		}
		cur.Close()
	}()

	if err := netns.Set(handle); err != nil {
		return fmt.Errorf("failed to set netns: %w", err)
	}

	if err := fn(); err != nil {
		return fmt.Errorf("fn failed: %w", err)
	}

	return nil
}
