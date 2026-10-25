package ipc

const instructions = `sie is a terminal text editor the user is working in. These tools act on the buffer open in it, including unsaved changes, so you and the user edit the same text live. Use them when the user asks about the file open in sie, their selection, or unsaved changes; use your normal file tools for everything else.

Coordinates: lines and columns are 1-based. A column counts grapheme clusters. A range's end is exclusive. The position (last line + 1, column 1) is the end of the buffer. Text comes back with LF line endings (sie keeps the file's own endings).

Revisions: every state and text result has a revision. Edits must pass the revision you last read; if the user typed in between you get stale_revision and nothing changes, so read again. Each edit is one undo step for the user (Ctrl+Z). Edits are not saved; the user decides when to save.

Invalid UTF-8: text that is not valid UTF-8 comes back with escaped=true, where an invalid byte is written \xNN and a backslash \\. Send escaped=true with such text to write it back unchanged.

The first call asks the user to confirm in the editor (y/n/a); wait for them. Large buffers need a line range for get_text.`

type schema = map[string]any

func obj(required []string, props schema) schema {
	s := schema{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

var (
	integer = func(desc string) schema { return schema{"type": "integer", "description": desc} }
	str     = func(desc string) schema { return schema{"type": "string", "description": desc} }
	boolean = func(desc string) schema { return schema{"type": "boolean", "description": desc} }
	pos     = obj([]string{"line", "col"}, schema{
		"line": integer("1-based line"), "col": integer("1-based column, in grapheme clusters"),
	})
	revision = integer("the revision you last read; the edit fails with stale_revision if the buffer changed since")
	escaped  = boolean("true if text uses the escaped form (\\xNN for invalid bytes, \\\\ for a backslash)")
)

func tool(name, desc string, input schema) schema {
	return schema{"name": name, "description": desc, "inputSchema": input}
}

var tools = []schema{
	tool("list_sessions", "List the running sie editors: pid, file, language, unsaved changes, last activity, and whether a client is attached.", obj(nil, schema{})),
	tool("attach", "Ask the user, in the editor, to let you control its buffer. Without pid, uses the editor used most recently. Returns the buffer state. Other tools call this for you when needed.",
		obj(nil, schema{"pid": integer("process ID from list_sessions")})),
	tool("detach", "Let go of the editor.", obj(nil, schema{})),
	tool("get_state", "Path, language, encoding, line ending, unsaved changes, line count, cursors and selections (positions only), and the revision.", obj(nil, schema{})),
	tool("get_text", "Text of the buffer, including unsaved changes. Pass a line range for large buffers (at most 2000 lines per call).",
		obj(nil, schema{"start_line": integer("first line, 1-based"), "end_line": integer("last line, inclusive")})),
	tool("get_selection", "The user's selections with their text; an empty selection is a cursor. The first is the primary one.", obj(nil, schema{})),
	tool("goto", "Move the cursor; the user sees it move.", pos),
	tool("select", "Select a range; the user sees it.", obj([]string{"start", "end"}, schema{"start": pos, "end": pos})),
	tool("insert", "Insert text at a position. Returns the new revision.",
		obj([]string{"line", "col", "text", "revision"}, schema{
			"line": integer("1-based line"), "col": integer("1-based column, in grapheme clusters"),
			"text": str("text to insert"), "escaped": escaped, "revision": revision})),
	tool("replace_range", "Replace a range with text (empty text deletes). Returns the new revision.",
		obj([]string{"start", "end", "text", "revision"}, schema{
			"start": pos, "end": pos, "text": str("replacement"), "escaped": escaped, "revision": revision})),
	tool("replace_selection", "Replace the user's selection (or insert at their cursor): use it for \"fix this\". Returns the new revision.",
		obj([]string{"text", "revision"}, schema{"text": str("replacement"), "escaped": escaped, "revision": revision})),
	tool("find", "Find every occurrence of a literal text. Returns ranges.",
		obj([]string{"pattern"}, schema{
			"pattern": str("text to find"), "case_sensitive": boolean("default false"),
			"limit": integer("most matches to return; default 100, at most 1000")})),
	tool("validate", "Check the buffer as its language (JSON, YAML, TOML, XML): valid, or the first problem with line and column.", obj(nil, schema{})),
	tool("run_command", "Run an editor prompt command such as \"goto 42\", \"trim trailing spaces\" or \"replace all old new\". Commands that touch files, quit, or ask questions are refused. See list_commands.",
		obj([]string{"command"}, schema{"command": str("the prompt line")})),
	tool("list_commands", "The prompt commands run_command accepts, with their arguments.", obj(nil, schema{})),
}
