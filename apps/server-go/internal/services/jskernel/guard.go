package jskernel

import (
	"fmt"

	"github.com/MoeForge/moejs"
)

// guardUnknownMembers replaces each object in the method tree with a Proxy
// whose get trap throws a message listing the names that exist, so a misspelled
// method or namespace is fixable from the error alone. Reads of names the
// engine itself probes (then, toJSON, Symbol keys) fall through untouched, so
// printing, JSON and Promise detection behave as before.
func (r *Runtime) guardUnknownMembers(rootName string) error {
	script := fmt.Sprintf(`
		(function(path) {
			function wrap(obj, curPath) {
				return new Proxy(obj, {
					get(target, prop, receiver) {
						if (typeof prop === "symbol" || prop in target || prop === "then" || prop === "toJSON" || prop === "constructor") {
							const v = Reflect.get(target, prop, receiver);
							return (v && typeof v === "object") ? wrap(v, curPath + "." + String(prop)) : v;
						}
						const keys = Object.keys(target).sort();
						throw new Error(curPath + "." + String(prop) + " does not exist. Available on " + curPath + ": " + keys.join(", "));
					}
				});
			}
			if (globalThis[path]) {
				globalThis[path] = wrap(globalThis[path], path);
			}
		})(%q);
	`, rootName)
	s, err := moejs.CompileScript("guard.js", script)
	if err != nil {
		return err
	}
	_, err = r.rt.RunScript(s)
	return err
}
