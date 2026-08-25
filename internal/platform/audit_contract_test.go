package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAuditWriteErrorsAreNeverDiscarded(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test location")
	}
	internalRoot := filepath.Dir(filepath.Dir(currentFile))
	err := filepath.Walk(internalRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return walkErr
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(node ast.Node) bool {
			assignment, isAssignment := node.(*ast.AssignStmt)
			if !isAssignment || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
				return true
			}
			blank, isBlank := assignment.Lhs[0].(*ast.Ident)
			call, isCall := assignment.Rhs[0].(*ast.CallExpr)
			if !isBlank || blank.Name != "_" || !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if isSelector && selector.Sel.Name == "AppendAudit" {
				t.Errorf("%s discards an AppendAudit error", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
