package expression

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker/decls"
	"google.golang.org/protobuf/types/known/structpb"
)

func env() (*cel.Env, error) {
	return cel.NewEnv(cel.Declarations(
		decls.NewVar("method", decls.String),
		decls.NewVar("path", decls.String),
		decls.NewVar("query", decls.Dyn),
		decls.NewVar("headers", decls.Dyn),
		decls.NewVar("body", decls.Dyn),
		decls.NewVar("config", decls.Dyn),
	))
}

func Validate(expression string) error {
	env, err := env()
	if err != nil {
		return err
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return issues.Err()
	}
	_, err = env.Program(ast)
	return err
}

func Evaluate(expression string, vars map[string]any) (bool, error) {
	env, err := env()
	if err != nil {
		return false, err
	}
	ast, issues := env.Compile(expression)
	if issues != nil && issues.Err() != nil {
		return false, issues.Err()
	}
	prog, err := env.Program(ast)
	if err != nil {
		return false, err
	}
	normalized := map[string]any{}
	for k, v := range vars {
		if m, ok := v.(map[string]any); ok {
			structVal, err := structpb.NewStruct(m)
			if err != nil {
				return false, err
			}
			normalized[k] = structVal
			continue
		}
		normalized[k] = v
	}
	out, _, err := prog.Eval(normalized)
	if err != nil {
		return false, err
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("expression did not evaluate to a boolean")
	}
	return b, nil
}
