package main

import (
	"fmt"
	"os"
	"os/exec"
)

// depsManifest maps runtime → list of known dependency file names (in priority order).
var depsManifest = map[string][]string{
	"javascript": {"package.json", "deno.json"},
	"typescript": {"package.json", "deno.json"},
	"python":     {"requirements.txt", "pyproject.toml"},
	"go":         {"go.mod"},
}

// detectDepsFile returns the dependency manifest file for the current project, or "".
func detectDepsFile(runtime string) string {
	for _, f := range depsManifest[runtime] {
		if _, err := os.Stat(f); err == nil {
			return f
		}
	}
	// Python: if .venv exists but no manifest, generate requirements.txt from venv
	if runtime == "python" {
		if _, err := os.Stat(".venv"); err == nil {
			if req := freezeVenv(); req != "" {
				os.WriteFile("requirements.txt", []byte(req), 0o644)
				fmt.Println("✓ Generated requirements.txt from .venv")
				return "requirements.txt"
			}
		}
	}
	return ""
}

// freezeVenv exports installed packages from .venv as requirements.txt content.
func freezeVenv() string {
	// Try uv first, then pip
	for _, args := range [][]string{
		{"uv", "pip", "freeze", "--python", ".venv/bin/python"},
		{".venv/bin/python", "-m", "pip", "freeze"},
	} {
		if _, err := exec.LookPath(args[0]); err != nil {
			continue
		}
		out, err := exec.Command(args[0], args[1:]...).Output()
		if err == nil && len(out) > 0 {
			return string(out)
		}
	}
	return ""
}

// detectPackageManager returns the best package manager for the runtime.
// Checks wkr.yaml config first, then lockfile heuristics, then defaults.
func detectPackageManager(cfg *WorkerConfig) string {
	if cfg.PackageManager != "" {
		return cfg.PackageManager
	}
	switch cfg.Runtime {
	case "javascript", "typescript":
		// Lockfile detection
		if _, err := os.Stat("deno.lock"); err == nil {
			return "deno"
		}
		if _, err := os.Stat("deno.json"); err == nil {
			return "deno"
		}
		if _, err := os.Stat("pnpm-lock.yaml"); err == nil {
			return "pnpm"
		}
		if _, err := os.Stat("yarn.lock"); err == nil {
			return "yarn"
		}
		if _, err := os.Stat("bun.lockb"); err == nil {
			return "bun"
		}
		return "npm"
	case "python":
		if _, err := exec.LookPath("uv"); err == nil {
			return "uv"
		}
		return "pip"
	case "go":
		return "go"
	}
	return ""
}

// installLocalDeps detects and installs dependencies locally before dev/deploy.
// Returns the manifest file content (to send to server) and the file name.
func installLocalDeps(cfg *WorkerConfig) (manifestContent string, manifestFile string) {
	manifestFile = detectDepsFile(cfg.Runtime)
	if manifestFile == "" {
		return "", ""
	}

	data, err := os.ReadFile(manifestFile)
	if err != nil {
		return "", ""
	}
	manifestContent = string(data)

	pm := detectPackageManager(cfg)
	if pm == "" {
		return manifestContent, manifestFile
	}

	// Check if install is needed
	if !needsInstall(cfg.Runtime, pm) {
		return manifestContent, manifestFile
	}

	fmt.Printf("Installing dependencies (%s)...\n", pm)

	// Python: ensure .venv exists before installing
	if (cfg.Runtime == "python") {
		if _, err := os.Stat(".venv"); err != nil {
			fmt.Println("Creating virtual environment...")
			var venvCmd *exec.Cmd
			if pm == "uv" {
				venvCmd = exec.Command("uv", "venv", ".venv")
			} else {
				pyBin := resolveLocalRuntime("python", cfg.RuntimeVersion)
				venvCmd = exec.Command(pyBin, "-m", "venv", ".venv")
			}
			venvCmd.Stdout = os.Stderr
			venvCmd.Stderr = os.Stderr
			if err := venvCmd.Run(); err != nil {
				fatal("failed to create venv: " + err.Error())
			}
		}
	}

	var cmd *exec.Cmd
	switch pm {
	case "npm":
		cmd = exec.Command("npm", "install", "--no-audit", "--no-fund")
	case "yarn":
		cmd = exec.Command("yarn", "install", "--frozen-lockfile")
	case "pnpm":
		cmd = exec.Command("pnpm", "install", "--frozen-lockfile")
	case "bun":
		cmd = exec.Command("bun", "install")
	case "deno":
		cmd = exec.Command("deno", "install")
	case "pip":
		if _, err := os.Stat(".venv"); err == nil {
			cmd = exec.Command(".venv/bin/python", "-m", "pip", "install", "-q", "-r", "requirements.txt")
		} else {
			cmd = exec.Command("pip", "install", "-q", "-r", "requirements.txt")
		}
	case "uv":
		if _, err := os.Stat(".venv"); err == nil {
			cmd = exec.Command("uv", "pip", "install", "-q", "--python", ".venv/bin/python", "-r", "requirements.txt")
		} else {
			cmd = exec.Command("uv", "pip", "install", "-q", "-r", "requirements.txt")
		}
	case "go":
		cmd = exec.Command("go", "mod", "download")
	default:
		return manifestContent, manifestFile
	}

	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("dependency install failed: " + err.Error())
	}
	fmt.Println("✓ Dependencies installed")

	// Re-read manifest in case lockfile generation changed it (e.g. deno adds to deno.json)
	if updated, err := os.ReadFile(manifestFile); err == nil {
		manifestContent = string(updated)
	}

	return manifestContent, manifestFile
}

// needsInstall checks if dependencies need to be installed.
func needsInstall(runtime, pm string) bool {
	switch runtime {
	case "javascript", "typescript":
		switch pm {
		case "deno":
			// deno manages its own cache; always run deno install to be safe
			return true
		default:
			// Check if node_modules exists
			if info, err := os.Stat("node_modules"); err != nil || !info.IsDir() {
				return true
			}
			// Check if lockfile is newer than node_modules
			nmInfo, _ := os.Stat("node_modules")
			for _, lf := range []string{"package-lock.json", "yarn.lock", "pnpm-lock.yaml", "package.json"} {
				if lfInfo, err := os.Stat(lf); err == nil {
					if lfInfo.ModTime().After(nmInfo.ModTime()) {
						return true
					}
				}
			}
			return false
		}
	case "python":
		// Install if requirements.txt exists (pip/uv are fast for already-installed)
		if _, err := os.Stat("requirements.txt"); err != nil {
			return false
		}
		// If .venv exists and is newer than requirements.txt, skip
		if vInfo, err := os.Stat(".venv"); err == nil {
			if rInfo, err := os.Stat("requirements.txt"); err == nil {
				if vInfo.ModTime().After(rInfo.ModTime()) {
					return false
				}
			}
		}
		return true
	case "go":
		if _, err := os.Stat("go.sum"); err != nil {
			return true
		}
		return false
	}
	return false
}
