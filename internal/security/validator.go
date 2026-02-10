package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/cubetiqlabs/wkr/internal/config"
)

// Validator checks worker code for dangerous patterns before execution.
type Validator struct {
	cfg config.SecurityConfig
}

func NewValidator(cfg config.SecurityConfig) *Validator {
	return &Validator{cfg: cfg}
}

// ValidateCode checks code size, blocked imports/globals, and integrity.
func (v *Validator) ValidateCode(code, runtime, codeHash string) error {
	if len(code) > v.cfg.MaxCodeSizeBytes {
		return fmt.Errorf("code size %d exceeds limit %d bytes", len(code), v.cfg.MaxCodeSizeBytes)
	}

	if v.cfg.CodeSigningKey != "" && codeHash != "" {
		if !v.verifyIntegrity(code, codeHash) {
			return fmt.Errorf("code integrity check failed")
		}
	}

	switch runtime {
	case "go":
		return v.validateGo(code)
	case "javascript", "typescript":
		return v.validateJS(code)
	}
	return nil
}

func (v *Validator) validateGo(code string) error {
	// Default dangerous imports if none configured
	blocked := v.cfg.BlockedImports
	if len(blocked) == 0 {
		blocked = []string{
			"os/exec", "syscall", "unsafe", "plugin",
			"net/http/pprof", "runtime/debug",
		}
	}
	for _, imp := range blocked {
		// Check for both quoted import and dot-import
		if strings.Contains(code, `"`+imp+`"`) {
			return fmt.Errorf("blocked import: %s", imp)
		}
	}

	// Block direct file operations if FS disabled
	if v.cfg.FSDisabled {
		for _, fn := range []string{"os.Create", "os.Open", "os.Remove", "os.Mkdir", "os.WriteFile", "os.ReadFile"} {
			if strings.Contains(code, fn) {
				return fmt.Errorf("filesystem access blocked: %s", fn)
			}
		}
	}
	return nil
}

func (v *Validator) validateJS(code string) error {
	blocked := v.cfg.BlockedJSGlobals
	if len(blocked) == 0 {
		blocked = []string{
			"Deno.run", "Deno.Command", "Deno.execPath",
			"child_process", "require('fs')", "require(\"fs\")",
			"Deno.writeFile", "Deno.readFile", "Deno.remove",
			"Deno.mkdir", "Deno.writeTextFile",
		}
	}
	for _, g := range blocked {
		if strings.Contains(code, g) {
			return fmt.Errorf("blocked operation: %s", g)
		}
	}

	if v.cfg.FSDisabled {
		for _, fn := range []string{"writeFileSync", "readFileSync", "mkdirSync", "unlinkSync"} {
			if strings.Contains(code, fn) {
				return fmt.Errorf("filesystem access blocked: %s", fn)
			}
		}
	}
	return nil
}

func (v *Validator) verifyIntegrity(code, expectedHash string) bool {
	mac := hmac.New(sha256.New, []byte(v.cfg.CodeSigningKey))
	mac.Write([]byte(code))
	computed := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(computed), []byte(expectedHash))
}

// SignCode produces an HMAC-SHA256 signature for code integrity verification.
func (v *Validator) SignCode(code string) string {
	if v.cfg.CodeSigningKey == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(v.cfg.CodeSigningKey))
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}
