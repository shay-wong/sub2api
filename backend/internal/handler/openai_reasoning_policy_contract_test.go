package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

// Every OpenAI ingress path must derive reasoning policy after scheduler selection.
func TestOpenAIIngressPathsUseEffectiveReasoningPolicy(t *testing.T) {
	tests := []struct {
		file                 string
		method               string
		helperCalls          []string
		forbiddenHelperCalls []string
	}{
		{
			file:                 "openai_chat_completions.go",
			method:               "ChatCompletions",
			helperCalls:          []string{"bindRequestedReasoningEffort", "ApplyEffectiveOpenAIReasoningEffortPolicy"},
			forbiddenHelperCalls: []string{"applyOpenAIReasoningEffortPolicyForRequest"},
		},
		{
			file:                 "openai_gateway_handler.go",
			method:               "Responses",
			helperCalls:          []string{"bindRequestedReasoningEffort", "ApplyEffectiveOpenAIReasoningEffortPolicy"},
			forbiddenHelperCalls: []string{"applyOpenAIReasoningEffortPolicyForRequest"},
		},
		{
			file:   "openai_gateway_handler.go",
			method: "ResponsesWebSocket",
			helperCalls: []string{
				"EffectiveOpenAIReasoningEffortPolicy",
				"ApplyEffectiveOpenAIReasoningEffortPolicy",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), tt.file, nil, 0)
			require.NoError(t, err)

			method := findFunctionDeclaration(file, tt.method)
			require.NotNil(t, method, "handler method must exist")
			for _, helperCall := range tt.helperCalls {
				require.True(t, functionCallsIdentifier(method, helperCall),
					"%s must use %s for the scheduler-selected group's reasoning policy", tt.method, helperCall)
			}
			for _, helperCall := range tt.forbiddenHelperCalls {
				require.False(t, functionCallsIdentifier(method, helperCall),
					"%s must not apply %s before scheduler selection", tt.method, helperCall)
			}
		})
	}
}

func findFunctionDeclaration(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

func functionCallsIdentifier(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if ok && ident.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}
