package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/parser"
	"go/token"
	"strings"

	"github.com/cubetiqlabs/wkr/internal/config"
)

// Validator checks worker code for dangerous patterns before execution.
type Validator struct {
	cfg            config.SecurityConfig
	blockedImports map[string]struct{}
	blockedJS      []string
}

func NewValidator(cfg config.SecurityConfig) *Validator {
	blocked := cfg.BlockedImports
	if len(blocked) == 0 {
		blocked = []string{
			"os/exec", "syscall", "unsafe", "plugin",
			"net/http/pprof", "runtime/debug",
		}
	}
	bm := make(map[string]struct{}, len(blocked))
	for _, imp := range blocked {
		bm[imp] = struct{}{}
	}

	blockedJS := cfg.BlockedJSGlobals
	if len(blockedJS) == 0 {
		blockedJS = []string{
			"Deno.run", "Deno.Command", "Deno.execPath",
			"child_process", "require('fs')", `require("fs")`,
			"Deno.writeFile", "Deno.readFile", "Deno.remove",
			"Deno.mkdir", "Deno.writeTextFile",
		}
	}

	return &Validator{cfg: cfg, blockedImports: bm, blockedJS: blockedJS}
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

// validateGo uses the Go AST parser to reliably detect blocked imports,
// preventing bypass via aliased imports or string manipulation.
func (v *Validator) validateGo(code string) error {
	// Wrap in a minimal compilable file for AST parsing
	wrapped := "package main\n" + code
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", wrapped, parser.ImportsOnly)
	if err != nil {
		// If AST parse fails, fall back to string scan (code may use raw function body)
		return v.validateGoFallback(code)
	}

	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if _, blocked := v.blockedImports[path]; blocked {
			return fmt.Errorf("blocked import: %s", path)
		}
	}

	// Also block go:linkname directive which can bypass import restrictions
	if strings.Contains(code, "go:linkname") {
		return fmt.Errorf("blocked directive: go:linkname")
	}

	if v.cfg.FSDisabled {
		for _, fn := range []string{"os.Create", "os.Open", "os.Remove", "os.Mkdir", "os.WriteFile", "os.ReadFile"} {
			if strings.Contains(code, fn) {
				return fmt.Errorf("filesystem access blocked: %s", fn)
			}
		}
	}
	return nil
}

// validateGoFallback is used when AST parsing fails (user code is a function body, not a full file).
func (v *Validator) validateGoFallback(code string) error {
	for imp := range v.blockedImports {
		if strings.Contains(code, `"`+imp+`"`) {
			return fmt.Errorf("blocked import: %s", imp)
		}
	}
	if strings.Contains(code, "go:linkname") {
		return fmt.Errorf("blocked directive: go:linkname")
	}
	return nil
}

func (v *Validator) validateJS(code string) error {
	for _, g := range v.blockedJS {
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
