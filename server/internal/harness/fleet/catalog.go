// Package fleet owns Aycorn's fixed agent definitions. Only models are stored
// as user preferences; prompts and skill contracts ship with the application.
package fleet

import (
	"embed"
	"fmt"
	"strconv"
)

//go:embed *.toml *-instructions.md skills
var Files embed.FS

// Definition is one bundled agent. Internal agents (Conductor, Chatter) run
// Aycorn itself: users never pick them, and they always use the workspace's
// default model. The rest are task agents, which Conductor dispatches and
// users can also start directly on a task.
type Definition struct {
	Role, Name, Description string
	ReadOnly                bool
	Internal                bool
}

var definitions = []Definition{
	{Role: "conductor", Name: "Conductor", Description: "Default project orchestrator: selects managed tasks and starts independent sessions through MCP.", ReadOnly: true, Internal: true},
	{Role: "coder", Name: "Coder", Description: "Implements assigned work and verifies the resulting behavior."},
	{Role: "reviewer", Name: "Reviewer", Description: "Independently reviews changes and validation against the requirements.", ReadOnly: true},
	{Role: "researcher", Name: "Research", Description: "Resolves technical questions using repository evidence and primary sources.", ReadOnly: true},
	{Role: "chatter", Name: "Chatter", Description: "Persistent project conversation and scoped task coordination.", ReadOnly: true, Internal: true},
}

func All() []Definition { return append([]Definition(nil), definitions...) }

// IsTaskRole reports whether role is a task agent Conductor may dispatch.
func IsTaskRole(role string) bool {
	d, ok := Lookup(role)
	return ok && !d.Internal
}

// IsInternalRole reports whether role is one of Aycorn's own internal agents.
func IsInternalRole(role string) bool {
	d, ok := Lookup(role)
	return ok && d.Internal
}

func Lookup(role string) (Definition, bool) {
	for _, d := range definitions {
		if d.Role == role {
			return d, true
		}
	}
	return Definition{}, false
}

func (d Definition) Instructions() string {
	b, err := Files.ReadFile(d.Role + "-instructions.md")
	if err != nil {
		panic(err)
	} // Missing bundled assets are a build defect.
	return string(b)
}

const WorkflowPath = "skills/aycorn-workflow/SKILL.md"

func Workflow() string {
	b, err := Files.ReadFile(WorkflowPath)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// Config always derives the developer instructions from the same Markdown the
// AI page displays. Models are immutable snapshots for the current run.
func (d Definition) Config(model, skillPath string) []byte {
	config := fmt.Sprintf("name = %s\ndescription = %s\ndeveloper_instructions = %s\n", strconv.Quote(d.Role), strconv.Quote(d.Description), strconv.Quote(d.Instructions()))
	if model != "" {
		config += "model = " + strconv.Quote(model) + "\n"
	}
	if d.ReadOnly {
		config += "sandbox_mode = \"read-only\"\n"
	}
	config += "\n[agents]\nenabled = false\n"
	config += "\n[[skills.config]]\npath = " + strconv.Quote(skillPath) + "\nenabled = true\n"
	return []byte(config)
}
