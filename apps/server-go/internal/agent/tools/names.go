package tools

// toolAliases maps alternative naming styles (e.g. legacy snake_case) to their
// canonical camelCase tool name.
var toolAliases = map[string]string{
	"read_file":           "readFile",
	"write_file":          "writeFile",
	"list_dir":            "listDir",
	"read_chat":           "readChat",
	"browser_reset":       "browserReset",
	"console_reset":       "consoleReset",
	"computer_reset":      "computerReset",
	"code_reset":          "codeReset",
	"edit_file":           "editFile",
	"batch_write":         "batchWrite",
	"bash_job":            "bashJob",
	"web_fetch":           "webFetch",
	"web_search":          "webSearch",
	"read_skill":          "readSkill",
	"load_tools":          "loadTools",
	"write_plan":          "writePlan",
	"request_full_access": "requestFullAccess",
	"ask_many":            "askMany",
}

// CanonicalName returns the standardized camelCase tool name for any known variant.
func CanonicalName(name string) string {
	if canon, ok := toolAliases[name]; ok {
		return canon
	}
	return name
}

// AliasesFor returns all accepted alternative names for a given tool name.
func AliasesFor(name string) []string {
	canon := CanonicalName(name)
	var aliases []string
	if canon != name {
		aliases = append(aliases, canon)
	}
	for alias, c := range toolAliases {
		if c == canon && alias != name {
			aliases = append(aliases, alias)
		}
	}
	return aliases
}
