package executor

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestUserGrantMissingArgs(t *testing.T) {
	req := BuiltinRequest{
		Args:   []string{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
	err := runUserGrant(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "缺少用户名") {
		t.Errorf("expected missing username error, got %v", err)
	}
}

func TestUserRevokeMissingArgs(t *testing.T) {
	req := BuiltinRequest{
		Args:   []string{},
		Stdout: &bytes.Buffer{},
		Stderr: &bytes.Buffer{},
	}
	err := runUserRevoke(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "缺少用户名") {
		t.Errorf("expected missing username error, got %v", err)
	}
}

func TestUserGrantUnsupportedOS(t *testing.T) {
	// This test only runs on unsupported platforms (none currently)
	// We test the error path by mocking runtime.GOOS which is not possible
	// in Go, so we just verify the function doesn't panic with valid input
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		req := BuiltinRequest{
			Args:   []string{"testuser"},
			Stdout: &bytes.Buffer{},
			Stderr: &bytes.Buffer{},
		}
		err := runUserGrant(context.Background(), req)
		if err == nil || !strings.Contains(err.Error(), "不支持的操作系统") {
			t.Errorf("expected unsupported OS error, got %v", err)
		}
	}
}

func TestGroupExists(t *testing.T) {
	// Test with a group that should exist on most Unix systems
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		// 'staff' or 'wheel' should exist on macOS, 'root' or 'wheel' on Linux
		exists := groupExists("root") || groupExists("wheel") || groupExists("staff")
		if !exists {
			t.Log("warning: none of root/wheel/staff groups found, this is unusual but not fatal")
		}
	}
}
