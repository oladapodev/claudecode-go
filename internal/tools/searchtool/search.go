package searchtool

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/oladapodev/claudecode-go/internal/domain"
)

type globInput struct {
	Pattern string `json:"pattern"`
	Root    string `json:"root,omitempty"`
}

type grepInput struct {
	Pattern string `json:"pattern"`
	Root    string `json:"root,omitempty"`
	Limit   int    `json:"limit,omitempty"`
}

type tool struct {
	name        string
	description string
	schema      json.RawMessage
	run         func(context.Context, domain.ToolInvocation) (domain.ToolResult, error)
}

func NewGlobTool() domain.Tool {
	return tool{
		name:        "glob",
		description: "Find files matching a glob pattern.",
		schema:      json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"root":{"type":"string"}},"required":["pattern"]}`),
		run: func(_ context.Context, invocation domain.ToolInvocation) (domain.ToolResult, error) {
			var input globInput
			if err := json.Unmarshal(invocation.Input, &input); err != nil {
				return domain.ToolResult{}, err
			}
			root, err := searchRoot(invocation.CWD, input.Root)
			if err != nil {
				return domain.ToolResult{}, err
			}

			matches := []string{}
			err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				if ok, _ := filepath.Match(input.Pattern, rel); ok {
					matches = append(matches, rel)
				}
				return nil
			})
			if err != nil {
				return domain.ToolResult{}, err
			}

			return domain.ToolResult{Content: strings.Join(matches, "\n")}, nil
		},
	}
}

func NewGrepTool() domain.Tool {
	return tool{
		name:        "grep",
		description: "Search files for a regular expression.",
		schema:      json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"root":{"type":"string"},"limit":{"type":"integer"}},"required":["pattern"]}`),
		run: func(_ context.Context, invocation domain.ToolInvocation) (domain.ToolResult, error) {
			var input grepInput
			if err := json.Unmarshal(invocation.Input, &input); err != nil {
				return domain.ToolResult{}, err
			}
			root, err := searchRoot(invocation.CWD, input.Root)
			if err != nil {
				return domain.ToolResult{}, err
			}
			if input.Limit <= 0 {
				input.Limit = 100
			}

			re, err := regexp.Compile(input.Pattern)
			if err != nil {
				return domain.ToolResult{}, err
			}

			lines := []string{}
			err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() || len(lines) >= input.Limit {
					return err
				}

				file, err := os.Open(path)
				if err != nil {
					return nil
				}
				defer file.Close()

				scanner := bufio.NewScanner(file)
				lineNo := 0
				for scanner.Scan() {
					lineNo++
					text := scanner.Text()
					if re.MatchString(text) {
						rel, _ := filepath.Rel(root, path)
						lines = append(lines, rel+":"+itoa(lineNo)+": "+text)
						if len(lines) >= input.Limit {
							break
						}
					}
				}
				return nil
			})
			if err != nil {
				return domain.ToolResult{}, err
			}

			return domain.ToolResult{Content: strings.Join(lines, "\n")}, nil
		},
	}
}

func (t tool) Name() string { return t.name }

func (t tool) Schema() json.RawMessage { return t.schema }

func (t tool) Description() string { return t.description }

func (t tool) Authorize(ctx context.Context, invocation domain.ToolInvocation, resolver domain.PermissionResolver) (domain.ToolAuthorization, error) {
	var target string
	var err error
	switch t.name {
	case "glob":
		var input globInput
		if err = json.Unmarshal(invocation.Input, &input); err != nil {
			return domain.ToolAuthorization{}, err
		}
		target, err = searchRoot(invocation.CWD, input.Root)
	case "grep":
		var input grepInput
		if err = json.Unmarshal(invocation.Input, &input); err != nil {
			return domain.ToolAuthorization{}, err
		}
		target, err = searchRoot(invocation.CWD, input.Root)
	}
	if err != nil {
		return domain.ToolAuthorization{}, err
	}
	var decision domain.PermissionDecision
	if withinWorkspace(invocation.CWD, target) {
		decision, err = resolver.Resolve(ctx, t.name, target, "searching within the workspace", domain.PermissionAllow)
	} else {
		decision, err = resolver.Resolve(ctx, t.name, target, "searching outside the workspace requires confirmation", domain.PermissionAsk)
	}
	if err != nil {
		return domain.ToolAuthorization{}, err
	}
	return domain.ToolAuthorization{Target: target, Decision: decision}, nil
}

func (t tool) Run(ctx context.Context, invocation domain.ToolInvocation) (domain.ToolResult, error) {
	return t.run(ctx, invocation)
}

func searchRoot(cwd, root string) (string, error) {
	if root == "" {
		return cwd, nil
	}
	if filepath.IsAbs(root) {
		return filepath.Clean(root), nil
	}
	return filepath.Clean(filepath.Join(cwd, root)), nil
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func withinWorkspace(cwd, root string) bool {
	cwd = filepath.Clean(cwd)
	root = filepath.Clean(root)
	return root == cwd || strings.HasPrefix(root, cwd+string(filepath.Separator))
}
