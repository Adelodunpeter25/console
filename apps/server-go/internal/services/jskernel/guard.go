package jskernel

import (
	"fmt"
	"sort"

	"github.com/dop251/goja"
)

// guardUnknownMembers replaces each object in the method tree with a Proxy
// whose get trap throws a message listing the names that exist, so a misspelled
// method or namespace is fixable from the error alone. Reads of names the
// engine itself probes (then, toJSON, Symbol keys) fall through untouched, so
// printing, JSON and Promise detection behave as before.
func (r *Runtime) guardUnknownMembers(root *goja.Object, rootName string) error {
	helper, err := r.rt.RunString(`(function (target, path, names, wrap) {
		return new Proxy(target, {
			get(obj, key, receiver) {
				if (typeof key === "symbol" || key in obj || key === "then" || key === "toJSON" || key === "constructor") {
					const v = Reflect.get(obj, key, receiver);
					return (v && typeof v === "object") ? wrap(v, path + "." + String(key)) : v;
				}
				throw new Error(path + "." + String(key) + " does not exist. Available on " + path + ": " + names(obj).join(", "));
			}
		});
	})`)
	if err != nil {
		return err
	}
	build, ok := goja.AssertFunction(helper)
	if !ok {
		return fmt.Errorf("could not install the unknown-member guard")
	}
	listNames := func(call goja.FunctionCall) goja.Value {
		obj, _ := call.Arguments[0].(*goja.Object)
		keys := obj.Keys()
		sort.Strings(keys)
		return r.rt.ToValue(keys)
	}
	var wrap func(call goja.FunctionCall) goja.Value
	wrap = func(call goja.FunctionCall) goja.Value {
		target := call.Arguments[0]
		path := call.Arguments[1].String()
		out, werr := build(goja.Undefined(), target, r.rt.ToValue(path), r.rt.ToValue(listNames), r.rt.ToValue(wrap))
		if werr != nil {
			panic(r.rt.NewGoError(werr))
		}
		return out
	}
	guarded, werr := build(goja.Undefined(), root, r.rt.ToValue(rootName), r.rt.ToValue(listNames), r.rt.ToValue(wrap))
	if werr != nil {
		return werr
	}
	return r.rt.Set(rootName, guarded)
}
