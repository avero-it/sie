// Package commands is the single list of user-facing commands. Every entry
// point (shortcut, menu, prompt) refers to commands by ID; the app runs them
// through one Dispatch. Commands are generic over the environment E they run
// against, so this package does not depend on the app.
package commands

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Arg describes one argument of a command. Missing arguments are asked for
// in the prompt, labelled with Prompt and pre-filled with Default.
type Arg[E any] struct {
	Prompt  string
	Default func(env E) string
}

// Command is one user-facing action.
type Command[E any] struct {
	ID      string   // namespaced, e.g. "file.save"
	Title   string   // shown in menus, prompt completions and the echo
	Aliases []string // extra names for the prompt, e.g. "goto"
	Args    []Arg[E]
	// Silent commands are not echoed or kept in history (e.g. opening the prompt).
	Silent bool
	// Checked makes the command an on/off option and reports whether it is
	// on; menus, completions and the echo show a tick for it. Nil for
	// ordinary commands.
	Checked func(env E) bool
	Run     func(env E, args []string) error
}

// Names returns the lower-cased title and aliases used to match prompt input.
func (c *Command[E]) Names() []string {
	names := []string{strings.ToLower(c.Title)}
	for _, a := range c.Aliases {
		names = append(names, strings.ToLower(a))
	}
	return names
}

// Registry holds commands in registration order with unique IDs.
type Registry[E any] struct {
	cmds []*Command[E]
	byID map[string]*Command[E]
}

// NewRegistry returns an empty registry.
func NewRegistry[E any]() *Registry[E] {
	return &Registry[E]{byID: map[string]*Command[E]{}}
}

// Register adds a command; IDs must be unique and every command needs a
// title and a Run function.
func (r *Registry[E]) Register(c Command[E]) error {
	switch {
	case c.ID == "" || c.Title == "" || c.Run == nil:
		return fmt.Errorf("register %q: ID, Title and Run are required", c.ID)
	case r.byID[c.ID] != nil:
		return fmt.Errorf("register %q: duplicate command ID", c.ID)
	}
	cmd := c
	r.cmds = append(r.cmds, &cmd)
	r.byID[c.ID] = &cmd
	return nil
}

// RegisterAll registers commands, collecting all errors.
func (r *Registry[E]) RegisterAll(cs ...Command[E]) error {
	var errs []error
	for _, c := range cs {
		errs = append(errs, r.Register(c))
	}
	return errors.Join(errs...)
}

// Get returns the command with the given ID.
func (r *Registry[E]) Get(id string) (*Command[E], bool) {
	c, ok := r.byID[id]
	return c, ok
}

// All returns the commands sorted by title.
func (r *Registry[E]) All() []*Command[E] {
	out := slices.Clone(r.cmds)
	slices.SortFunc(out, func(a, b *Command[E]) int { return strings.Compare(a.Title, b.Title) })
	return out
}

// ParseInline matches prompt input against command names: either a whole
// name ("replace all") or "<name> <args>" for commands that take arguments
// ("goto 42", "find foo bar"). The longest matching name wins, so "replace
// all x y" is Replace all, not Replace with args "all x y". The last argument
// takes the rest of the line; earlier ones are single words.
func (r *Registry[E]) ParseInline(input string) (cmd *Command[E], args []string, ok bool) {
	input = strings.TrimSpace(input)
	best := 0
	for _, c := range r.cmds {
		for _, name := range c.Names() {
			if len(name) <= best || len(input) < len(name) || !strings.EqualFold(input[:len(name)], name) {
				continue
			}
			whole := len(input) == len(name)
			withArgs := len(c.Args) > 0 && len(input) > len(name) && input[len(name)] == ' '
			if whole || withArgs {
				best, cmd = len(name), c
			}
		}
	}
	if cmd == nil {
		return nil, nil, false
	}
	rest := strings.TrimSpace(input[best:])
	for i := 0; i < len(cmd.Args)-1 && rest != ""; i++ {
		word, tail, _ := strings.Cut(rest, " ")
		args = append(args, word)
		rest = strings.TrimSpace(tail)
	}
	if rest != "" {
		args = append(args, rest)
	}
	return cmd, args, true
}
