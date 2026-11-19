package htmlrender

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Theme is the set of CSS variables a page may use. Values match Console's
// desktop/mobile palette (console-ui/src/theme/mod.rs); clients can override
// them live via postMessage when the user switches themes.
type Theme struct {
	Appearance string            `json:"appearance"` // "dark" | "light"
	Variables  map[string]string `json:"variables"`
}

// DefaultThemes mirror Console's dark and light palettes.
var DefaultThemes = map[string]Theme{
	"dark": {
		Appearance: "dark",
		Variables: map[string]string{
			"--background": "#000000", "--foreground": "#e2e2e2",
			"--muted": "#151515", "--muted-foreground": "#a3a3a3",
			"--card": "#1a1a1a", "--card-foreground": "#e2e2e2",
			"--popover": "#0e0e0e", "--popover-foreground": "#e2e2e2",
			"--secondary": "#232323", "--secondary-foreground": "#e2e2e2",
			"--border": "#2a2a2a", "--input": "#2a2a2a", "--ring": "#e2795b",
			"--primary": "#e2795b", "--primary-foreground": "#ffffff",
			"--accent": "#e2795b", "--accent-foreground": "#ffffff",
			"--destructive": "#e2726a", "--destructive-foreground": "#ffffff",
			"--warning": "#e0b36a", "--warning-foreground": "#1a1a1a",
			"--success": "#62c987", "--success-foreground": "#1a1a1a",
			"--info": "#3b82f6", "--info-foreground": "#ffffff",
			"--code-background": "#151515", "--code-foreground": "#e0a882",
			"--chart-1": "#e2795b", "--chart-2": "#3b82f6", "--chart-3": "#62c987",
			"--chart-4": "#e0b36a", "--chart-5": "#b583e8", "--chart-6": "#4fc3c7",
			"--chart-7": "#e86fa6", "--chart-8": "#9aa64b",
			"--radius": "0.625rem",
			"--font-sans": "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif",
			"--font-mono": "'SF Mono', 'SFMono-Regular', Menlo, Consolas, monospace",
		},
	},
	"light": {
		Appearance: "light",
		Variables: map[string]string{
			"--background": "#f6f5f6", "--foreground": "#242424",
			"--muted": "#e6e6e6", "--muted-foreground": "#666666",
			"--card": "#f6f5f6", "--card-foreground": "#242424",
			"--popover": "#e6e6e6", "--popover-foreground": "#242424",
			"--secondary": "#ececec", "--secondary-foreground": "#242424",
			"--border": "#d8d8dc", "--input": "#d8d8dc", "--ring": "#c85f44",
			"--primary": "#c85f44", "--primary-foreground": "#ffffff",
			"--accent": "#c85f44", "--accent-foreground": "#ffffff",
			"--destructive": "#c64a42", "--destructive-foreground": "#ffffff",
			"--warning": "#a66b20", "--warning-foreground": "#ffffff",
			"--success": "#2f8f52", "--success-foreground": "#ffffff",
			"--info": "#2563eb", "--info-foreground": "#ffffff",
			"--code-background": "#ececec", "--code-foreground": "#9a5528",
			"--chart-1": "#c85f44", "--chart-2": "#2563eb", "--chart-3": "#2f9e5b",
			"--chart-4": "#b8862f", "--chart-5": "#8b5cc6", "--chart-6": "#1f9aa0",
			"--chart-7": "#c8407f", "--chart-8": "#6e7a2a",
			"--radius": "0.625rem",
			"--font-sans": "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif",
			"--font-mono": "'SF Mono', 'SFMono-Regular', Menlo, Consolas, monospace",
		},
	},
}

// ThemeFor returns the theme for an appearance name, defaulting to dark.
func ThemeFor(appearance string) Theme {
	if t, ok := DefaultThemes[strings.ToLower(appearance)]; ok {
		return t
	}
	return DefaultThemes["dark"]
}

// ThemeFragment encodes a theme for the #console-theme= URL fragment the
// bootstrap reads before first paint.
func ThemeFragment(t Theme) string {
	raw, _ := json.Marshal(t)
	return "#console-theme=" + url.QueryEscape(string(raw))
}

// BootstrapMarkup is the head markup injected at serve time: charset/viewport
// meta, the base theme stylesheet, and the bridge script. Generated per request
// so the file on disk stays clean.
func BootstrapMarkup(t Theme) string {
	vars := ""
	for k, v := range t.Variables {
		vars += k + ":" + v + ";"
	}
	css := fmt.Sprintf(`:root{color-scheme:%s;%s}`, t.Appearance, vars) +
		`html{box-sizing:border-box;padding:8px;background:var(--background);color:var(--foreground);font-family:var(--font-sans);font-size:14px;line-height:1.5}` +
		`body{margin:0}code,kbd,pre,samp{font-family:var(--font-mono)}` +
		`:where(:focus-visible){outline:2px solid var(--ring)}`
	return `<meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width, initial-scale=1">` +
		`<style id="console-theme">` + css + `</style>` +
		`<script>` + bootstrapScript() + `</script>`
}

// bootstrapScript installs the page→host bridge: theme updates over
// postMessage, auto-height reporting, link forwarding, and the console error
// hook preview reads back. Runs before page scripts via <head> injection.
func bootstrapScript() string {
	return `(function(){
var s=document.getElementById("console-theme"),n=0;
window.__consoleErrors=[];
function push(l,m){try{window.__consoleErrors.push({level:l,text:String(m).slice(0,500)});if(window.__consoleErrors.length>50)window.__consoleErrors.shift();}catch(e){}}
["error","warn"].forEach(function(k){var o=console[k];console[k]=function(){var a=[].slice.call(arguments);push(k,a.join(" "));return o.apply(console,a);};});
window.addEventListener("error",function(e){push("error",(e.message||"script error")+" @"+(e.filename||"")+":"+(e.lineno||0));});
window.addEventListener("unhandledrejection",function(e){push("error","unhandledrejection: "+(e.reason&&(e.reason.message||e.reason)));});
function a(t){if(!t||typeof t!=="object"||!t.variables||typeof t.variables!=="object")return;var c=":root{color-scheme:"+(t.appearance==="light"?"light":"dark")+";";for(var k in t.variables){if(/^--[a-z0-9-]+$/i.test(k))c+=k+":"+String(t.variables[k]).replace(/[;{}<>]/g,"")+";";}s.textContent=c+"}";}
try{var m=/[#&]console-theme=([^&]*)/.exec(location.hash);if(m&&s){var t=JSON.parse(decodeURIComponent(m[1]));a(t);history.replaceState(history.state,"",location.pathname+location.search);}}catch(e){}
window.addEventListener("message",function(e){var d=e.data,p=d&&d.params;if(d&&d.jsonrpc==="2.0"&&d.method==="console/theme"&&p&&p.variables)a({appearance:p.appearance,variables:p.variables});});
document.addEventListener("click",function(e){var l=e.isTrusted?e.composedPath().find(function(t){return t&&t.matches&&t.matches("a[href]");}):null,u;if(!l)return;try{u=new URL(l.getAttribute("href"),document.baseURI);}catch(x){return;}if(!/^https?:$/.test(u.protocol)||u.href.split("#")[0]===location.href.split("#")[0])return;if(window.parent!==window){e.preventDefault();window.parent.postMessage({jsonrpc:"2.0",id:"console-link-"+(++n),method:"console/open-link",params:{url:u.href,metaKey:e.metaKey,ctrlKey:e.ctrlKey}},"*");}else{l.setAttribute("target","_blank");l.setAttribute("rel","noopener");}},true);
var h,o,z=function(){var r=document.documentElement,v=Math.ceil(r.scrollHeight>r.clientHeight?r.scrollHeight:r.getBoundingClientRect().height);if(v===h)return;h=v;var m={jsonrpc:"2.0",method:"console/size-changed",params:{height:v}};if(window.parent!==window)window.parent.postMessage(m,"*");try{if(window.ipc&&window.ipc.postMessage)window.ipc.postMessage(JSON.stringify(m));}catch(e){}try{if(window.ConsoleHost&&window.ConsoleHost.postMessage)window.ConsoleHost.postMessage(JSON.stringify(m));}catch(e){}};
if(window.ResizeObserver){o=new ResizeObserver(z);o.observe(document.documentElement);}document.addEventListener("DOMContentLoaded",function(){if(o&&document.body)o.observe(document.body);z();});window.addEventListener("load",z);
})();`
}
