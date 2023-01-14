package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func loadEnv(key, defaultValue string) string {
	value := os.Getenv(key)

	if value != "" {
		return value
	}

	return defaultValue
}

func loadEnvBool(key string, defaultValue bool) bool {
	value := loadEnv(key, strconv.FormatBool(defaultValue))

	b, err := strconv.ParseBool(value)

	if err != nil {
		log.Fatalf("%s is invalid: %s", key, value)
	}

	return b
}

func copyFiles(src, dst string) error {
	matches, err := filepath.Glob(filepath.Join(src, "*"))

	if err != nil {
		return fmt.Errorf("failed to list files in %s: %w", src, err)
	}

	cmd := exec.Command("cp", append(matches, dst)...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func copyPlugins() error {
	src := loadEnv("PLUGIN_DIR_SRC", "/plugins")
	dst := loadEnv("PLUGIN_DIR_DST", "/opt/cni/bin/")

	if err := copyFiles(src, dst); err != nil {
		return fmt.Errorf("failed to copy from %s to %s: %w", src, dst, err)
	}

	return nil
}

func copyConf() error {
	src := loadEnv("CONFIG_DIR_SRC", "/config")
	dst := loadEnv("CONFIG_DIR_DST", "/etc/cni/net.d/")

	if err := copyFiles(src, dst); err != nil {
		return fmt.Errorf("failed to copy from %s to %s: %w", src, dst, err)
	}

	return nil
}

func main() {
	if !loadEnvBool("SKIP_COPY_PLUGINS", false) {
		if err := copyPlugins(); err != nil {
			log.Fatalf("failed to copy plugins: %+v", err)
		}
	}

	if !loadEnvBool("SKIP_COPY_CONFIG", false) {
		if err := copyConf(); err != nil {
			log.Fatalf("failed to copy plugins: %+v", err)
		}
	}

	cmd := exec.Command(os.Args[1], os.Args[2:]...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	err := cmd.Run()

	exitCode := 1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	if err != nil {
		log.Printf("%+v", err)
		os.Exit(exitCode)
	}
}
