// Package sourcecheck implements the strict v1 source admission check for
// trusted native plugins. It reduces accidental privilege use; it is not a
// sandbox and cannot make same-process Go code untrusted.
package sourcecheck

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Finding struct {
	Path    string
	Line    int
	Message string
}

func (f Finding) String() string {
	if f.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Message)
	}
	return fmt.Sprintf("%s: %s", f.Path, f.Message)
}

var forbiddenImports = map[string]string{
	"C":            "cgo is outside the native plugin v1 host contract",
	"database/sql": "plugins must use invocation-scoped State instead of database handles",
	"io/ioutil":    "plugins must not use deprecated direct filesystem helpers",
	"log":          "plugins must use Host.Logger so configured secrets can be redacted",
	"log/slog":     "plugins must use Host.Logger so configured secrets can be redacted",
	"net":          "plugins must use Host.HTTP instead of direct networking",
	"net/http":     "plugins must use Host.HTTP instead of direct networking",
	"os":           "plugins must use registered configuration and State instead of process environment or filesystem access",
	"os/exec":      "process execution is outside the native plugin v1 host contract",
	"plugin":       "dynamic Go plugins are not supported",
	"syscall":      "direct system calls are outside the native plugin v1 host contract",
	"unsafe":       "unsafe code is outside the native plugin v1 host contract",
}

var forbiddenExtensions = map[string]bool{
	".7z": true, ".a": true, ".bin": true, ".c": true, ".cc": true,
	".class": true, ".cpp": true, ".dll": true, ".dylib": true, ".exe": true,
	".gz": true, ".h": true, ".jar": true, ".lib": true, ".o": true,
	".obj": true, ".s": true, ".so": true, ".syso": true, ".tar": true,
	".wasm": true, ".zip": true,
}

const (
	maxSourceFileBytes = 1 << 20
	maxSourceTreeBytes = 8 << 20
	maxSourceFiles     = 512
)

func Check(root string) ([]Finding, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("plugin source root must be a real directory")
	}
	findings := make([]Finding, 0)
	fset := token.NewFileSet()
	files, totalBytes := 0, int64(0)
	err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			findings = append(findings, Finding{Path: path, Message: "symlinks are not allowed in plugin source"})
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if path != absolute && entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		files++
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		totalBytes += info.Size()
		if files > maxSourceFiles || totalBytes > maxSourceTreeBytes {
			findings = append(findings, Finding{Path: path, Message: "plugin source tree exceeds the 512-file or 8 MiB review limit"})
			return fs.SkipAll
		}
		if info.Size() > maxSourceFileBytes {
			findings = append(findings, Finding{Path: path, Message: "plugin source file exceeds the 1 MiB review limit"})
			return nil
		}
		extension := strings.ToLower(filepath.Ext(path))
		if forbiddenExtensions[extension] {
			findings = append(findings, Finding{Path: path, Message: "compiled, foreign, or assembler files are not allowed"})
			return nil
		}
		if extension != ".go" {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
				findings = append(findings, Finding{Path: path, Message: "non-Go plugin package files must be human-readable UTF-8 text"})
			}
			return nil
		}
		parsed, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			findings = append(findings, Finding{Path: path, Message: "Go source cannot be parsed: " + parseErr.Error()})
			return nil
		}
		for _, declaration := range parsed.Decls {
			if function, ok := declaration.(*ast.FuncDecl); ok && function.Recv == nil && function.Name.Name == "init" {
				findings = append(findings, Finding{Path: path, Line: fset.Position(function.Pos()).Line, Message: "init functions are forbidden; registration and lifecycle must be explicit"})
			}
		}
		for _, imported := range parsed.Imports {
			name, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				continue
			}
			reason := forbiddenImportReason(name)
			if reason != "" {
				findings = append(findings, Finding{Path: path, Line: fset.Position(imported.Pos()).Line, Message: fmt.Sprintf("forbidden import %q: %s", name, reason)})
			}
		}
		file, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		scanner := bufio.NewScanner(file)
		line := 0
		for scanner.Scan() {
			line++
			if strings.Contains(scanner.Text(), "//go:linkname") {
				findings = append(findings, Finding{Path: path, Line: line, Message: "go:linkname is outside the native plugin v1 host contract"})
			}
		}
		closeErr := file.Close()
		if scanErr := scanner.Err(); scanErr != nil {
			return scanErr
		}
		return closeErr
	})
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Message < findings[j].Message
	})
	return findings, err
}

func forbiddenImportReason(path string) string {
	if reason := forbiddenImports[path]; reason != "" {
		return reason
	}
	if strings.Contains(path, "/internal/") {
		return "plugins must use the public nativeplugin SDK instead of DokoSoko internals"
	}
	if strings.HasPrefix(path, "golang.org/x/sys") {
		return "direct system calls are outside the native plugin v1 host contract"
	}
	if strings.HasPrefix(path, "github.com/jackc/pgx") || strings.Contains(path, "sqlite") {
		return "plugins must use invocation-scoped State instead of database packages"
	}
	return ""
}
