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
	// Normalize pip3 → pip
	if pm == "pip3" {
		pm = "pip"
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
		// Determine manifest filename: deno.json for Deno import maps, package.json otherwise
		manifestFile := "package.json"
		if pm == "deno" || strings.Contains(deps, `"imports"`) {
			manifestFile = "deno.json"
		}
		if err := os.WriteFile(filepath.Join(dir, manifestFile), []byte(deps), 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", manifestFile, err)
		}
		switch pm {
		case "deno":
			cmd = exec.CommandContext(ctx, "deno", "install")
		case "yarn":
			cmd = exec.CommandContext(ctx, "yarn", "install", "--production")
		case "pnpm":
			cmd = exec.CommandContext(ctx, "pnpm", "install", "--prod")
		case "bun":
			cmd = exec.CommandContext(ctx, "bun", "install", "--production")
		default: // npm
			cmd = exec.CommandContext(ctx, "npm", "install", "--omit=dev", "--no-audit", "--no-fund")
		}
	case "python":
		if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(deps), 0o644); err != nil {
			return "", fmt.Errorf("write requirements.txt: %w", err)
		}
		reqFile := filepath.Join(dir, "requirements.txt")
		venvDir := filepath.Join(dir, ".venv")
		homeEnv := "HOME=" + os.Getenv("HOME")

		// Create venv if it doesn't exist
		if _, err := os.Stat(filepath.Join(venvDir, "bin", "python")); err != nil {
			os.RemoveAll(venvDir) // clean partial state
			created := false
			if pm == "uv" {
				venvCmd := exec.CommandContext(ctx, "uv", "venv", venvDir, "--python", runtimeBin)
				venvCmd.Dir = dir
				venvCmd.Env = append(env, homeEnv)
				if out, err := venvCmd.CombinedOutput(); err == nil {
					created = true
				} else {
					// uv venv failed (OOM/killed) — fall back to stdlib venv
					os.RemoveAll(venvDir)
					_ = out
				}
			}
			if !created {
				venvCmd := exec.CommandContext(ctx, runtimeBin, "-m", "venv", venvDir)
				venvCmd.Dir = dir
				venvCmd.Env = append(env, homeEnv)
				if out, err := venvCmd.CombinedOutput(); err != nil {
					return "", fmt.Errorf("create venv failed: %s\n%s", err, string(out))
				}
			}
		}

		venvPython := filepath.Join(venvDir, "bin", "python")
		if pm == "uv" {
			// Try uv first, fall back to pip on failure
			cmd = exec.CommandContext(ctx, "uv", "pip", "install", "-q", "--python", venvPython, "-r", reqFile)
			cmd.Dir = dir
			cmd.Env = append(env, homeEnv, "VIRTUAL_ENV="+venvDir)
			if out, err := cmd.CombinedOutput(); err != nil {
				// uv killed/OOM — fall back to pip inside venv
				cmd = exec.CommandContext(ctx, venvPython, "-m", "pip", "install", "-q", "--no-input", "-r", reqFile)
			} else {
				_ = out
				os.WriteFile(marker, []byte("ok"), 0o644)
				return dir, nil
			}
		} else {
			cmd = exec.CommandContext(ctx, venvPython, "-m", "pip", "install", "-q", "--no-input", "-r", reqFile)
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

// PreInstallDeps warms the dependency cache so the first invoke doesn't pay install cost.
func (e *SandboxEngine) PreInstallDeps(ctx context.Context, rt, version, deps, pm string) {
	if strings.TrimSpace(deps) == "" {
		return
	}
	bin := resolveRuntime(rt, version)
	installDeps(ctx, rt, deps, pm, bin, e.hostEnv)
}
