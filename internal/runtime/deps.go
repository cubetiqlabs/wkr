package runtime

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// resolveRuntime returns the binary path for a given runtime and version.
// If version is empty, falls back to the system default.
func resolveRuntime(rt, version string) string {
	if version == "" {
		return defaultBinary(rt)
	}
	switch rt {
	case "go":
		if bin, err := exec.LookPath("go" + version); err == nil {
			return bin
		}
	case "python":
		if bin, err := exec.LookPath("python" + version); err == nil {
			return bin
		}
	case "javascript", "typescript":
		if bin, err := exec.LookPath("node" + version); err == nil {
			return bin
		}
	}
	return defaultBinary(rt)
}

func defaultBinary(rt string) string {
	switch rt {
	case "go":
		return "go"
	case "python":
		return "python3"
	case "javascript", "typescript":
		if _, err := exec.LookPath("deno"); err == nil {
			return "deno"
		}
		return "node"
	}
	return rt
}

var (
	depsCacheBase = filepath.Join(os.TempDir(), "cubis-deps")
	depsCacheInit sync.Once
)

func ensureDepsCacheDir() {
	depsCacheInit.Do(func() { os.MkdirAll(depsCacheBase, 0o700) })
}

func depsHash(pm, deps string) string {
	h := sha256.Sum256([]byte(pm + "\x00" + deps))
	return fmt.Sprintf("%x", h[:16])
}

// defaultPM returns a default package manager for the runtime if none specified.
func defaultPM(rt string) string {
	switch rt {
	case "javascript", "typescript":
		if _, err := exec.LookPath("deno"); err == nil {
			return "deno"
		}
		return "npm"
	case "python":
		return "pip"
	case "go":
		return "go"
	}
	return ""
}

// installDeps installs dependencies into a cached directory and returns the path.
// Returns ("", nil) if no dependencies are specified.
func installDeps(ctx context.Context, rt, deps, pm, runtimeBin string, env []string) (string, error) {
	if strings.TrimSpace(deps) == "" {
		return "", nil
	}
	ensureDepsCacheDir()

	if pm == "" {
		pm = defaultPM(rt)
	}

	key := depsHash(pm, deps)
	dir := filepath.Join(depsCacheBase, key)

	// Fast path: already installed
	marker := filepath.Join(dir, ".cubis-deps-ok")
	if _, err := os.Stat(marker); err == nil {
		return dir, nil
	}

	os.MkdirAll(dir, 0o755)

	var cmd *exec.Cmd
	switch rt {
	case "javascript", "typescript":
		switch pm {
		case "deno":
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(deps), 0o644); err != nil {
				return "", fmt.Errorf("write package.json: %w", err)
			}
			cmd = exec.CommandContext(ctx, "deno", "install")
		case "yarn":
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(deps), 0o644); err != nil {
				return "", fmt.Errorf("write package.json: %w", err)
			}
			cmd = exec.CommandContext(ctx, "yarn", "install", "--production")
		case "pnpm":
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(deps), 0o644); err != nil {
				return "", fmt.Errorf("write package.json: %w", err)
			}
			cmd = exec.CommandContext(ctx, "pnpm", "install", "--prod")
		case "bun":
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(deps), 0o644); err != nil {
				return "", fmt.Errorf("write package.json: %w", err)
			}
			cmd = exec.CommandContext(ctx, "bun", "install", "--production")
		default: // npm
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(deps), 0o644); err != nil {
				return "", fmt.Errorf("write package.json: %w", err)
			}
			cmd = exec.CommandContext(ctx, "npm", "install", "--omit=dev", "--no-audit", "--no-fund")
		}
	case "python":
		if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(deps), 0o644); err != nil {
			return "", fmt.Errorf("write requirements.txt: %w", err)
		}
		reqFile := filepath.Join(dir, "requirements.txt")
		switch pm {
		case "uv":
			cmd = exec.CommandContext(ctx, "uv", "pip", "install", "-q", "--target", dir, "-r", reqFile)
		default: // pip
			cmd = exec.CommandContext(ctx, runtimeBin, "-m", "pip", "install", "-q", "--target", dir, "-r", reqFile)
		}
	case "go":
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(deps), 0o644); err != nil {
			return "", fmt.Errorf("write go.mod: %w", err)
		}
		cmd = exec.CommandContext(ctx, runtimeBin, "mod", "download")
	default:
		return "", fmt.Errorf("deps not supported for runtime: %s", rt)
	}

	cmd.Dir = dir
	cmd.Env = append(env, "HOME="+os.Getenv("HOME"))

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("install deps failed (%s): %s\n%s", pm, err, string(out))
	}

	os.WriteFile(marker, []byte("ok"), 0o644)
	return dir, nil
}
