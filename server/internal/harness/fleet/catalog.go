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

type Definition struct {
	Role, Name, Description string
	ReadOnly                bool
}

var definitions = []Definition{
	{"conductor", "Conductor", "Default project orchestrator: selects managed tasks and starts independent sessions through MCP.", true},
	{"planner", "Planner", "Checks readiness, dependencies and acceptance criteria before implementation.", true},
	{"researcher", "Research", "Resolves technical questions using repository evidence and primary sources.", true},
	{"coder", "Coder", "Implements assigned work and verifies the resulting behavior.", false},
	{"reviewer", "Reviewer", "Independently reviews changes and validation against the requirements.", true},
	{"chatter", "Chatter", "Persistent project conversation and scoped task coordination.", true},
}

func All() []Definition { return append([]Definition(nil), definitions...) }

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
