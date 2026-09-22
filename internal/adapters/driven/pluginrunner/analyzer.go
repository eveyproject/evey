package pluginrunner

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/evey-project/evey/internal/domain/plugin"
)

type AnalysisSeverity int

const (
	SeverityInfo     AnalysisSeverity = 1
	SeverityWarning  AnalysisSeverity = 2
	SeverityCritical AnalysisSeverity = 3
)

type AnalysisFinding struct {
	Severity    AnalysisSeverity
	Category    string
	File        string
	Line        int
	Pattern     string
	Description string
}

type ExtractedURL struct {
	URL  string
	File string
	Line int
	Type string // "literal", "template", "concatenation"
}

type AnalysisReport struct {
	Findings          []AnalysisFinding
	URLs              []ExtractedURL
	TotalFilesScanned int
	Safe              bool
}

type analyzerRule struct {
	pattern     *regexp.Regexp
	severity    AnalysisSeverity
	category    string
	description string
	extensions  []string // empty = all
}

var analyzerRules = []analyzerRule{
	// Shell execution - qualified calls (clear intent, no false positives).
	{regexp.MustCompile(`(?i)(os\.system|subprocess\.(call|run|Popen)|child_process\.(exec|execSync|spawn|spawnSync))`), SeverityCritical, "shell_exec", "shell command execution", nil},
	// Bare `exec(` / `spawn(` - only when preceded by a non-member-access
	// boundary (space, start, ;, (, {, =, ,) so `regex.exec(`, `Class.spawn(`,
	// `pool.exec(` don't trigger. Case-sensitive on the bare identifier
	// because `Exec` / `Spawn` are class names in three.js / R3F.
	{regexp.MustCompile(`(?:^|[\s;(,{=])(?:exec|spawn)\s*\(`), SeverityCritical, "shell_exec", "bare exec/spawn call", nil},
	{regexp.MustCompile(`(?i)(os\.popen|commands\.getoutput|backtick)`), SeverityCritical, "shell_exec", "shell command execution via pipe", nil},
	// Node.js child_process import - flagged on the IMPORT itself, independent
	// of how the binding is used downstream. The qualified-call rule above
	// (`child_process\.(exec|...)`) only matches the literal token
	// `child_process.` right before the method, so `const cp =
	// require("child_process"); cp.exec(...)`, `require("child_process").spawnSync(...)`
	// and a stored method reference all slipped through with zero findings.
	// Bounding the module name to exact quotes (`["']child_process["']`)
	// keeps `require("child_process_logger")` from false-positiving.
	// String-concatenation evasion (`"child_" + "process"`) is already
	// covered by the obfuscation rule below - not duplicated here.
	{regexp.MustCompile(`(?i)require\s*\(\s*["']child_process["']\s*\)|import\s+[^;\n]*\bfrom\s+["']child_process["']|import\s*\(\s*["']child_process["']\s*\)`), SeverityCritical, "shell_exec", "Node.js child_process import (shell/process execution)", nil},

	// Overreach - a plugin trying to be infrastructure. Not necessarily
	// malicious: the recurring case is a well-meaning author who packaged a
	// whole application as a plugin (spins up Docker, installs packages at
	// runtime, reaches for its own database) instead of the small sandboxed
	// thing a plugin is. WARNING, not Critical - the admin must SEE it at
	// preview and decide, because none of it can work inside the sandbox and
	// the plugin will fail confusingly if installed blind.
	{regexp.MustCompile(`(?i)(docker\s+(run|compose|build|pull)|docker-compose|dockerode|["'` + "`" + `]dockerode["'` + "`" + `]|/var/run/docker\.sock)`), SeverityWarning, "overreach", "drives Docker - a plugin runs sandboxed, it cannot manage containers", nil},
	{regexp.MustCompile("(?i)((npm|pnpm|yarn)\\s+(install|add|i)\\s|(pip3?|apt(-get)?|apk|gem|go)\\s+(install|add|get)\\s|[\"'`](npm|pnpm|yarn|pip|pip3|apt|apt-get|apk|gem)[\"'`]\\s*,\\s*[\"'`](install|add|get|i)[\"'`])"), SeverityWarning, "overreach", "installs packages at runtime - dependencies must be vendored in the bundle, not fetched on the host", nil},
	{regexp.MustCompile("(?i)(postgres(ql)?|mysql|mongodb(\\+srv)?|redis|mariadb)://[^\\s\"'`]+"), SeverityWarning, "overreach", "connects to a database by URL - a plugin has no database of its own; use the host data RPCs", nil},
	{regexp.MustCompile(`(?i)(new\s+Pool\s*\(|new\s+Client\s*\(|mysql\.createConnection|mongoose\.connect|psycopg2\.connect|sqlalchemy\.create_engine|new\s+Sequelize\s*\()`), SeverityWarning, "overreach", "opens a database client - a plugin persists through the host data RPCs, not its own DB", nil},

	// Network - outbound connections.
	// Qualified network APIs first (always meaningful).
	{regexp.MustCompile(`(?i)(socket\.connect|http\.request|urllib\.request|requests\.(get|post|put|delete)|axios\.|net\.Dial)`), SeverityWarning, "network", "outbound network connection", nil},
	// Bare `fetch(` only when not a member access. Lowercase only -
	// `Fetch` / `prefetchFetch` / etc. are common identifiers in minified JS.
	{regexp.MustCompile(`(?:^|[\s;(,{=])fetch\s*\(`), SeverityWarning, "network", "outbound fetch() call", nil},
	{regexp.MustCompile(`(?i)(reverse.?shell|bind.?shell|nc\s+-[elp]|ncat\s|socat\s)`), SeverityCritical, "network", "potential reverse/bind shell", nil},

	// Filesystem - dangerous access
	{regexp.MustCompile(`(?i)(\/etc\/passwd|\/etc\/shadow|\.ssh\/|authorized_keys|id_rsa)`), SeverityCritical, "filesystem", "access to sensitive system files", nil},
	{regexp.MustCompile(`(?i)(shutil\.rmtree|rimraf|rm\s+-rf|os\.remove|unlink\()`), SeverityWarning, "filesystem", "recursive file deletion", nil},
	{regexp.MustCompile(`(?i)(symlink|os\.link|os\.symlink|fs\.symlinkSync)`), SeverityWarning, "filesystem", "symlink creation", nil},

	// Code obfuscation - `eval(` is always dangerous regardless of where it
	// appears. Python's `compile(src, fname, "exec")` is matched by requiring
	// an "exec"/'exec' string literal INSIDE the compile() parens, so chained
	// calls like `renderer.compile(L,V,j); regex.exec(...)` from minified
	// Three.js bundles don't trigger.
	{regexp.MustCompile(`(?i)(eval\(|compile\s*\([^)]*?['"]exec['"][^)]*\))`), SeverityCritical, "obfuscation", "dynamic code execution (eval/compile)", nil},
	{regexp.MustCompile(`(?i)(base64\.b64decode|atob\(|Buffer\.from\(.*base64)`), SeverityWarning, "obfuscation", "base64 decoding (potential obfuscated payload)", nil},
	{regexp.MustCompile(`\\x[0-9a-fA-F]{2}.*\\x[0-9a-fA-F]{2}.*\\x[0-9a-fA-F]{2}`), SeverityWarning, "obfuscation", "hex-encoded string sequence", nil},

	// Privilege escalation
	{regexp.MustCompile(`(?i)(setuid|setgid|chmod\s+[47]777|chmod\s+u\+s)`), SeverityCritical, "privilege", "privilege escalation attempt", nil},
	{regexp.MustCompile(`(?i)(sudo\s|su\s+-|doas\s)`), SeverityCritical, "privilege", "sudo/su usage", nil},

	// Crypto mining
	{regexp.MustCompile(`(?i)(stratum\+tcp|xmrig|coinhive|cryptonight|monero)`), SeverityCritical, "cryptomining", "crypto mining indicator", nil},

	// Package install at runtime
	{regexp.MustCompile(`(?i)(pip\s+install|npm\s+install|yarn\s+add|go\s+install|curl\s.*\|\s*(sh|bash))`), SeverityCritical, "runtime_install", "runtime package installation", nil},

	// Dynamic imports - bypass static import restrictions
	{regexp.MustCompile(`(?i)(__import__\s*\(|importlib\.import_module|getattr\s*\(\s*__builtins__)`), SeverityCritical, "dynamic_import", "dynamic module import (potential sandbox bypass)", nil},

	// Obfuscation via getattr(__builtins__, ...) - used to retrieve eval/exec by name
	{regexp.MustCompile(`getattr\s*\(\s*__builtins__\s*,`), SeverityCritical, "obfuscation_getattr_builtins", "getattr(__builtins__, ...) used to access builtins dynamically (sandbox bypass)", nil},

	// __import__ called with a variable argument instead of a string literal
	{regexp.MustCompile(`__import__\s*\(\s*[a-zA-Z_][a-zA-Z0-9_]*\s*[\),]`), SeverityCritical, "obfuscation_dynamic_import", "__import__ called with non-literal argument (dynamic module import)", nil},

	// Node.js sandbox escape
	{regexp.MustCompile(`(?i)(vm\.(runIn|createContext)|process\.binding\(|new\s+Function\s*\()`), SeverityCritical, "sandbox_escape", "potential sandbox escape via vm/Function/binding", nil},

	// Direct syscall interface
	{regexp.MustCompile(`(?i)(ctypes\.CDLL|ctypes\.cdll|ffi\.Library)`), SeverityCritical, "native_code", "native code execution via ctypes/ffi", nil},

	// String concatenation evasion - building dangerous strings dynamically
	{regexp.MustCompile(`(?i)require\s*\(\s*["'][^"']*["']\s*\+`), SeverityCritical, "obfuscation", "dynamic require with string concatenation", nil},
	// require with array.join evasion: require(["child", "_process"].join(""))
	{regexp.MustCompile(`(?i)require\s*\(\s*\[[^\]]+\]\s*\.\s*(?:join|reduce)\s*\(`), SeverityCritical, "obfuscation", "dynamic require with array.join evasion", nil},
	{regexp.MustCompile(`(?i)["']child_["']\s*\+|["']sub["']\s*\+\s*["']process`), SeverityCritical, "obfuscation", "obfuscated child_process/subprocess import", nil},
	{regexp.MustCompile(`(?i)["']pass["']\s*\+\s*["']wd|["']shad["']\s*\+\s*["']ow`), SeverityCritical, "obfuscation", "obfuscated sensitive file path construction", nil},
	// Assembling "kill" from fragments to evade the process.kill detector below.
	{regexp.MustCompile(`(?i)["']ki["']\s*\+\s*["']ll|["']kil["']\s*\+\s*["']l["']`), SeverityCritical, "obfuscation", "obfuscated kill() construction", nil},
	// JavaScript String.fromCharCode payload assembly (3+ codepoints).
	// Threshold of 3 args avoids hits on legitimate `String.fromCharCode(0xfeff)`
	// or single-char emit; obfuscators always produce long lists.
	{regexp.MustCompile(`String\.fromCharCode\s*\(\s*\d+\s*,\s*\d+\s*,\s*\d+`), SeverityWarning, "obfuscation", "String.fromCharCode chain (potential payload assembly)", nil},
	// Python: ''.join([chr(n), chr(m), ...]) - equivalent to chr-chain in PHP.
	{regexp.MustCompile(`(?i)["']{2}\.join\s*\(\s*\[?\s*chr\s*\(\s*\d+\s*\)\s*,\s*chr\s*\(`), SeverityWarning, "obfuscation", "Python chr-list join (potential string obfuscation)", nil},

	// Sensitive file paths - broader patterns
	{regexp.MustCompile(`(?i)/proc/\d+/environ`), SeverityCritical, "filesystem", "access to process environment via /proc", nil},
	{regexp.MustCompile(`(?i)/data/jwt\.key|jwt[._-]key|private[._-]key`), SeverityWarning, "filesystem", "reference to JWT/private key file", nil},
	{regexp.MustCompile(`(?i)\.env\.dev|\.env\.local|\.env\.prod`), SeverityWarning, "filesystem", "reference to environment config file", nil},

	// Process signals - direct call, plus bracket-notation evasion process["kill"](1
	{regexp.MustCompile(`(?i)process\.kill\s*\(\s*1[,)]|os\.kill\s*\(\s*1[,)]`), SeverityCritical, "privilege", "attempt to signal PID 1 (server process)", nil},
	{regexp.MustCompile(`(?i)process\s*\[\s*["']kill["']\s*\]\s*\(\s*1[,)]`), SeverityCritical, "privilege", "attempt to signal PID 1 via bracket-notation process[\"kill\"]", nil},

	// Python - unsafe deserialization (arbitrary code exec)
	{regexp.MustCompile(`(?i)(pickle\.loads?|pickle\.load\s*\(|marshal\.loads?|marshal\.load\s*\()`), SeverityCritical, "deserialization", "unsafe deserialization (pickle/marshal can execute arbitrary code)", nil},
	{regexp.MustCompile(`(?i)(yaml\.load\s*\([^,)]+\))`), SeverityWarning, "deserialization", "yaml.load without explicit Loader (use yaml.safe_load)", nil},
	{regexp.MustCompile(`(?i)__reduce__|__reduce_ex__`), SeverityCritical, "deserialization", "custom __reduce__ can hijack deserialization", nil},

	// Python - class hierarchy traversal (sandbox escape)
	{regexp.MustCompile(`(?i)(__class__\.__bases__|__subclasses__\s*\(\)|__mro__\b)`), SeverityCritical, "sandbox_escape", "class hierarchy traversal (common Python sandbox escape)", nil},

	// PHP - code execution patterns
	{regexp.MustCompile(`(?i)preg_replace\s*\(.*\/e`), SeverityCritical, "shell_exec", "PHP preg_replace /e modifier executes matched content as PHP code", nil},
	{regexp.MustCompile(`(?i)\bassert\s*\(\s*(base64_decode|gzinflate|str_rot13|\$_)`), SeverityCritical, "obfuscation", "PHP assert() with dynamic/encoded argument (code execution)", nil},
	{regexp.MustCompile(`(?i)(chr\s*\(\s*\d+\s*\)\s*\.){3,}`), SeverityWarning, "obfuscation", "PHP chr() chain (potential string obfuscation)", nil},
	// chr() chain WITHOUT explicit dot concat: chr(115) . chr(121) variant where
	// the writer drops the dot (PHP 8 short concat) or uses comma list inside an
	// implode/array. Catch 3+ chr() calls in close proximity regardless of
	// glue character - only (whitespace|.|,) allowed between them so we don't
	// over-match unrelated calls on the same line.
	{regexp.MustCompile(`(?i)chr\s*\(\s*\d+\s*\)\s*[\s.,]\s*chr\s*\(\s*\d+\s*\)\s*[\s.,]\s*chr\s*\(\s*\d+\s*\)`), SeverityWarning, "obfuscation", "PHP chr() chain (no-dot variant)", nil},
	{regexp.MustCompile(`(?i)str_rot13\s*\(`), SeverityWarning, "obfuscation", "PHP str_rot13 (common obfuscation technique)", nil},
	// PHP shell execution. `system\(` was triggering on `THREE.System(`,
	// `EffectSystem(`, etc. - capital S = class name, lowercase only. Also
	// require the call NOT be a member access (no `.system(`).
	{regexp.MustCompile(`(?:^|[^.\w])(?:system|passthru|shell_exec|popen|proc_open)\s*\(`), SeverityCritical, "shell_exec", "PHP shell execution function", []string{".php"}},

	// Ruby - command execution
	{regexp.MustCompile("(?i)(`[^`]*`)"), SeverityWarning, "shell_exec", "Ruby backtick command execution", []string{".rb"}},
	{regexp.MustCompile(`(?i)(Kernel\.(exec|system|spawn)\s*\(|%x\{|IO\.popen\s*\()`), SeverityCritical, "shell_exec", "Ruby Kernel.exec/system/spawn or IO.popen", []string{".rb"}},
	{regexp.MustCompile(`(?i)open\s*\(\s*["']\|`), SeverityCritical, "shell_exec", "Ruby open('|cmd') shell injection via pipe prefix", []string{".rb"}},

	// Go - unsafe pointer arithmetic
	{regexp.MustCompile(`(?i)\bunsafe\.Pointer\b`), SeverityWarning, "native_code", "Go unsafe.Pointer (bypasses type safety)", []string{".go"}},
	{regexp.MustCompile(`(?i)\bunsafe\.Slice\b|\bunsafe\.SliceData\b|\bunsafe\.StringData\b`), SeverityWarning, "native_code", "Go unsafe slice/string manipulation", []string{".go"}},

	// Shell - base64 pipe to shell (obfuscated payload)
	{regexp.MustCompile(`(?i)(base64\s+-d|base64\s+--decode|openssl\s+enc\s+-d)[^|]*\|\s*(sh|bash|dash|zsh|ash)\b`), SeverityCritical, "obfuscation", "base64-decoded payload piped to shell", nil},
	{regexp.MustCompile(`(?i)echo\s+[A-Za-z0-9+/]{20,}={0,2}\s*\|\s*(base64|openssl)`), SeverityCritical, "obfuscation", "inline base64 blob piped to decoder (obfuscated shell payload)", nil},

	// JS Function constructor escape - (function(){}).constructor("evil")()
	{regexp.MustCompile(`(?i)\.constructor\s*\(\s*["']`), SeverityCritical, "sandbox_escape", "Function via .constructor", nil},

	// Go raw syscall by number - syscall.Syscall(59, ...) bypasses all wrappers
	{regexp.MustCompile(`(?i)syscall\.(Syscall|RawSyscall)\s*\(\s*\d+`), SeverityCritical, "native_code", "raw syscall by number", []string{".go"}},

	// Python getattr alias on builtin - getattr(obj, "exec"/"eval"/...)
	{regexp.MustCompile(`(?i)getattr\s*\(\s*[a-zA-Z_]\w*\s*,\s*["'](exec|eval|system|__import__|compile)`), SeverityCritical, "dynamic_import", "getattr on builtin via alias", nil},

	// Python module aliasing - import os as _o; _o.system(...)
	{regexp.MustCompile(`(?i)(?:from\s+\S+\s+)?(?:os|subprocess|builtins|importlib)\s+(?:import\s+\w+\s+)?as\s+[a-zA-Z_]\w*`), SeverityWarning, "obfuscation", "module aliased - verify usage", nil},
	// Python aliased OS module used for shell execution on the same line.
	{regexp.MustCompile(`(?i)(?:os|subprocess)\s+as\s+[a-zA-Z_]\w*.*\.(system|popen|exec|call|run)\s*\(`), SeverityCritical, "shell_exec", "aliased OS module used for shell execution", nil},

	// Host secret access - CRITICAL (covers /data/*.key, /data/jwt, .aws/credentials, /run/secrets/, etc.)
	{regexp.MustCompile(`(?i)(/data/[^\s"'` + "`" + `]*\.(?:key|env|pem|p12|pfx)|/data/jwt|/etc/passwd|/etc/shadow|\.aws/credentials|/run/secrets/)`), SeverityCritical, "filesystem", "host secret access", nil},

	// __builtins__.__dict__["exec"] - dict bracket indexing to retrieve builtin
	// by name (literal). Critical if literal is dangerous.
	{regexp.MustCompile(`__builtins__\s*\.\s*__dict__\s*\[\s*["'](exec|eval|system|__import__|compile|open)["']\s*\]`), SeverityCritical, "dynamic_import", "__builtins__.__dict__ bracket indexing", nil},

	// __builtins__.__dict__[anything] - permissive variant. After alias
	// resolution (resolveSimpleAliases) literals get inlined; this catches
	// the unresolved case (variable name) so admin still sees a Warning.
	{regexp.MustCompile(`__builtins__\s*\.\s*__dict__\s*\[`), SeverityWarning, "dynamic_import", "__builtins__ dict access - verify", nil},

	// Stored .constructor reference followed by call: F(<ident>) where F is
	// known to be a Function constructor. The resolver rewrites F( to
	// .constructor( so the existing pattern at line 171 fires for literal-arg
	// calls; this fallback catches calls with a variable argument
	// (post-rewrite the line reads ".constructor(varname)").
	{regexp.MustCompile(`\.constructor\s*\(\s*[a-zA-Z_]\w*\s*\)`), SeverityCritical, "sandbox_escape", "Function.constructor call with variable arg", nil},
}

// URL extraction patterns - all network schemes
const urlSchemes = `(?:https?|ftp|ftps|wss?|ssh|telnet|ldaps?|smb|mqtt|amqp|redis|mongodb|postgres(?:ql)?|mysql|stratum\+tcp|gopher|file)`

var (
	urlLiteralRe  = regexp.MustCompile(urlSchemes + `://[^\s"'\x60\]}>),;]+`)
	urlTemplateRe = regexp.MustCompile(`(?:f["']|` + "`" + `)` + urlSchemes + `://[^"'` + "`" + `]+`)
	urlConcatRe   = regexp.MustCompile(`["']` + urlSchemes + `://["']\s*[\+\.]`)

	// Suspicious schemes that should always be flagged as WARNING
	suspiciousSchemeRe = regexp.MustCompile(`(?i)(wss?|ssh|telnet|ftp|ldap|smb|stratum\+tcp)://`)
)

var depManifests = map[string]bool{
	"requirements.txt": true, "Pipfile": true, "pyproject.toml": true,
	"package.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"composer.json": true, "Gemfile": true, "go.mod": true,
}

// isDepLockfile returns true for lockfiles whose URL extraction would
// flood the report with every transitive dependency's tarball.
func isDepLockfile(name string) bool {
	switch name {
	case "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		"Pipfile.lock", "poetry.lock", "composer.lock",
		"Gemfile.lock", "go.sum":
		return true
	}
	return false
}

// manifestDeclaresDeps best-effort scan: returns true when the manifest
// actually lists third-party dependencies. Empty / metadata-only manifests
// (e.g. a package.json with no "dependencies" field) don't trigger the
// unvendored_deps warning since they pull in nothing at runtime.
// hasVendoredModules reports whether a dependency tree sits beside a manifest -
// node_modules for npm, vendor for Go/PHP, or a populated site-packages. When
// one is present the "please vendor your deps" warning is a false positive
// (redteam-bridge ships node_modules and still tripped it).
func hasVendoredModules(dir string) bool {
	for _, name := range []string{"node_modules", "vendor"} {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func manifestDeclaresDeps(path string) bool {
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return true // can't read → fail closed
	}
	base := filepath.Base(path)
	switch base {
	case "package.json":
		// Match a non-empty "dependencies" or "peerDependencies" object.
		return depJSONRe.Match(data)
	case "composer.json":
		return depJSONRe.Match(data)
	case "requirements.txt":
		// Any non-empty, non-comment line counts as a declared dep.
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				return true
			}
		}
		return false
	case "go.mod":
		return strings.Contains(string(data), "require ")
	}
	// Lockfiles and other manifests: presence implies deps were resolved.
	return true
}

var depJSONRe = regexp.MustCompile(`(?s)"(?:dependencies|peerDependencies|require)"\s*:\s*\{[^}]*"`)

// Executable binary extensions - emit a warning (cannot be statically analyzed)
var executableExtensions = map[string]bool{
	".so": true, ".dll": true, ".dylib": true, ".exe": true,
	".bin": true, ".o": true, ".a": true, ".class": true, ".jar": true,
	".pyc": true, ".pyo": true, ".wasm": true,
}

// Executable binary extensions - emit a warning (cannot be statically analyzed)
var archiveExtensions = map[string]bool{
	".zip": true, ".gz": true, ".tar": true, ".bz2": true, ".xz": true,
}

// Asset extensions mapped to expected MIME type prefixes.
// DetectContentType returns "application/octet-stream" for unknown formats,
// so we accept that as valid for font/audio/doc types it can't identify.
var assetMIME = map[string][]string{
	".png":   {"image/png"},
	".jpg":   {"image/jpeg"},
	".jpeg":  {"image/jpeg"},
	".gif":   {"image/gif"},
	".ico":   {"image/"},
	".svg":   {"text/xml", "text/plain", "image/svg"},
	".woff":  {"application/octet-stream", "font/"},
	".woff2": {"application/octet-stream", "font/"},
	".ttf":   {"application/octet-stream", "font/"},
	".eot":   {"application/octet-stream", "font/"},
	".mp3":   {"audio/", "application/octet-stream"},
	".mp4":   {"video/", "application/octet-stream"},
	".wav":   {"audio/", "application/octet-stream"},
	".flac":  {"audio/", "application/octet-stream"},
	".pdf":   {"application/pdf"},
	".doc":   {"application/octet-stream", "application/msword"},
	".docx":  {"application/octet-stream", "application/zip"},
}

// StaticAnalyzer implements the domain PluginAnalyzer interface.
type StaticAnalyzer struct{}

func NewStaticAnalyzer() *StaticAnalyzer { return &StaticAnalyzer{} }

func (a *StaticAnalyzer) Analyze(dir, webRoot string) (*plugin.AnalysisReport, error) {
	raw, err := AnalyzePluginWithWebRoot(dir, webRoot)
	if err != nil {
		return nil, err
	}
	report := &plugin.AnalysisReport{
		Safe:      raw.Safe,
		FileCount: raw.TotalFilesScanned,
	}
	seenCat := map[string]bool{}
	for _, f := range raw.Findings {
		sev := severityLabel(f.Severity)
		report.Issues = append(report.Issues, fmt.Sprintf("[%s] %s:%d - %s (%s)", sev, f.File, f.Line, f.Description, f.Pattern))
		report.RawFindings = append(report.RawFindings, plugin.PluginFinding{
			Category:    f.Category,
			Severity:    sev,
			File:        f.File,
			Line:        f.Line,
			Description: f.Description,
		})
		if f.Category != "" && !seenCat[f.Category] {
			seenCat[f.Category] = true
			report.Categories = append(report.Categories, f.Category)
		}
	}
	for _, u := range raw.URLs {
		report.URLs = append(report.URLs, plugin.ExtractedURL{
			URL:  u.URL,
			File: u.File,
			Line: u.Line,
			Type: u.Type,
		})
	}
	return report, nil
}

func severityLabel(s AnalysisSeverity) string {
	switch s {
	case SeverityCritical:
		return "CRITICAL"
	case SeverityWarning:
		return "WARNING"
	default:
		return "INFO"
	}
}

// AnalyzePlugin scans a plugin directory for suspicious patterns.
// Equivalent to AnalyzePluginWithWebRoot(dir, "") - kept for tests/callers
// that don't carry a manifest.
func AnalyzePlugin(dir string) (*AnalysisReport, error) {
	return AnalyzePluginWithWebRoot(dir, "")
}

// AnalyzePluginWithWebRoot is the same as AnalyzePlugin: webRoot is no
// longer used to silence pattern rules (we tightened the regexes instead
// so `regex.exec(`, `Class.Fetch(`, `THREE.System(` etc. don't false-
// positive on minified bundles). Kept for callers that already pass the
// manifest's WebRoot - argument is accepted but unused.
func AnalyzePluginWithWebRoot(dir, _ string) (*AnalysisReport, error) {
	report := &AnalysisReport{Safe: true}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			base := filepath.Base(path)
			// .git holds no executed code, and __pycache__/.venv are build
			// detritus. node_modules and vendor, on the other hand, RUN - so
			// they are walked (see vendoredPath below), not skipped: a payload
			// buried in a dependency is still a payload.
			if base == ".git" || base == "__pycache__" || base == ".venv" {
				return filepath.SkipDir
			}
			return nil
		}

		// Warn when a dependency manifest declares deps that aren't vendored.
		// An empty/no-deps manifest (e.g. package.json with only {"main":"x.js"})
		// is fine - many plugins rely solely on the runtime's bundled SDK.
		base := info.Name()
		// A nested package.json INSIDE node_modules is a dependency's own
		// manifest, not the plugin's - its deps resolve up the tree, so the
		// "please vendor your deps" warning is meaningless noise there (16 of
		// them on redteam-bridge's grpc tree). Only the plugin's own top-level
		// manifest gets this check.
		relForManifest, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			relForManifest = path
		}
		if depManifests[base] && !vendoredPath(relForManifest) && manifestDeclaresDeps(path) && !hasVendoredModules(filepath.Dir(path)) {
			relPath, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				relPath = path
			}
			report.Findings = append(report.Findings, AnalysisFinding{
				Severity:    SeverityWarning,
				Category:    "unvendored_deps",
				File:        relPath,
				Line:        0,
				Pattern:     base,
				Description: "dependency manifest declares deps but no vendored modules found - bundle node_modules / vendor / etc. in the ZIP",
			})
		}

		// Skip auto-copied SDK stubs (protobuf generated code)
		if strings.HasSuffix(base, "_pb2.py") || strings.HasSuffix(base, "_pb2_grpc.py") || strings.HasSuffix(base, ".proto") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if expectedMIMEs, isAsset := assetMIME[ext]; isAsset {
			if mimeErr := verifyAssetMIME(path, ext, expectedMIMEs); mimeErr != "" {
				relPath, relErr := filepath.Rel(dir, path)
				if relErr != nil {
					relPath = path
				}
				report.TotalFilesScanned++
				report.Findings = append(report.Findings, AnalysisFinding{
					Severity:    SeverityWarning,
					Category:    "mime_mismatch",
					File:        relPath,
					Line:        0,
					Pattern:     ext,
					Description: mimeErr,
				})
			}
			return nil
		}
		if executableExtensions[ext] || archiveExtensions[ext] {
			relPath, relErr := filepath.Rel(dir, path)
			if relErr != nil {
				relPath = path
			}
			report.TotalFilesScanned++
			category := "compiled_binary"
			desc := "compiled binary cannot be statically analyzed - runtime sandbox will enforce isolation"
			if archiveExtensions[ext] {
				category = "embedded_archive"
				desc = "archive may contain unanalyzed code extractable at runtime"
			}
			report.Findings = append(report.Findings, AnalysisFinding{
				Severity:    SeverityWarning,
				Category:    category,
				File:        relPath,
				Line:        0,
				Pattern:     ext,
				Description: desc,
			})
			return nil
		}

		report.TotalFilesScanned++
		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			relPath = path
		}

		findings, urls, scanErr := scanFile(path, relPath)
		if scanErr != nil {
			return scanErr
		}
		// Vendored code runs, so it is scanned - but a legitimate library trips
		// every warning-level rule (fetch, base64, dynamic require) by the
		// hundred, and a report drowned in that noise is a report nobody reads.
		// Keep only what is never acceptable in a dependency either: the
		// Critical findings (shell exec, eval, raw syscall, obfuscated payload).
		if vendoredPath(relPath) {
			findings = vendoredCritical(findings)
			urls = nil
		}
		report.Findings = append(report.Findings, findings...)
		// Lockfiles list every transitive dep's tarball URL - that's not
		// signal, just noise. Keep findings (the unvendored_deps warning
		// fires on the manifest itself) but drop URL extraction here.
		if !isDepLockfile(base) {
			report.URLs = append(report.URLs, urls...)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk plugin dir: %w", err)
	}

	for _, f := range report.Findings {
		if f.Severity == SeverityCritical || f.Category == "mime_mismatch" {
			report.Safe = false
			break
		}
	}

	return report, nil
}

// vendoredPath reports whether relPath lives inside a bundled dependency tree.
// Such code runs but is not the plugin author's own, so it is held to the one
// bar that matters there: nothing Critical.
func vendoredPath(relPath string) bool {
	for seg := range strings.SplitSeq(filepath.ToSlash(relPath), "/") {
		if seg == "node_modules" || seg == "vendor" {
			return true
		}
	}
	return false
}

// vendoredCriticalDescriptions are the findings trustworthy enough to raise from
// inside a bundled dependency. The bar is deliberately high: a legitimately
// published library trips eval(), bare exec(), package.json install-scripts,
// native FFI and dynamic imports all the time (lodash, protobufjs, wrap-ansi,
// grpc-js all do), so raising those from vendored code would flag every plugin
// that ships a normal dependency tree - which is how the real backdoor would
// then hide in the noise. What remains is what a real library never contains by
// accident: process spawning, a reverse shell, a raw execve, a payload piped to
// a shell, a read of /etc/shadow, or a deliberate string-concat evasion.
var vendoredCriticalDescriptions = map[string]bool{
	"shell command execution":                                        true,
	"shell command execution via pipe":                               true,
	"Node.js child_process import (shell/process execution)":         true,
	"PHP shell execution function":                                   true,
	"Ruby Kernel.exec/system/spawn or IO.popen":                      true,
	"Ruby open('|cmd') shell injection via pipe prefix":              true,
	"aliased OS module used for shell execution":                     true,
	"potential reverse/bind shell":                                   true,
	"access to sensitive system files":                               true,
	"host secret access":                                             true,
	"access to process environment via /proc":                        true,
	"raw syscall by number":                                          true,
	"base64-decoded payload piped to shell":                          true,
	"inline base64 blob piped to decoder (obfuscated shell payload)": true,
	"obfuscated child_process/subprocess import":                     true,
	"obfuscated sensitive file path construction":                    true,
	"obfuscated kill() construction":                                 true,
}

// vendoredCritical keeps only the high-confidence critical findings (see
// vendoredCriticalDescriptions) from a scan of bundled dependency code.
func vendoredCritical(in []AnalysisFinding) []AnalysisFinding {
	var out []AnalysisFinding
	for _, f := range in {
		if f.Severity == SeverityCritical && vendoredCriticalDescriptions[f.Description] {
			out = append(out, f)
		}
	}
	return out
}

// verifyAssetMIME reads the first 512 bytes and checks the detected MIME type
// against expected prefixes. Returns an error description if mismatched, empty string if OK.
func verifyAssetMIME(path, ext string, expected []string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if n == 0 {
		return ""
	}

	detected := http.DetectContentType(buf[:n])
	for _, prefix := range expected {
		if strings.HasPrefix(detected, prefix) {
			return ""
		}
	}
	return fmt.Sprintf("file extension %s does not match detected content type %q - possible disguised file", ext, detected)
}

// docExtensions are prose, not executed code: a README that shows "docker run"
// as an example, or a CHANGELOG mentioning eval(), is describing, not doing.
// Running the code-pattern rules over them cries wolf (real false positives on
// the shipped example plugins, 2026-09-02), so scanFile skips the rules for
// these while still noting any URLs they carry.
var docExtensions = map[string]bool{
	".md": true, ".markdown": true, ".mdx": true, ".rst": true, ".txt": true, ".adoc": true,
}

func scanFile(path, relPath string) ([]AnalysisFinding, []ExtractedURL, error) {
	raw, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, nil, err
	}
	isDoc := docExtensions[strings.ToLower(filepath.Ext(path))]

	// File-level alias pre-pass: resolves simple variable/module bindings across
	// lines so that per-line rules can match patterns like $f = "system"; $f($x).
	content := resolveSimpleAliases(string(raw))

	// Strip /* … */ block comments for languages that use C-style block
	// comments. This must happen at the full-file level (before splitting into
	// lines) because block comments span multiple lines. Newlines inside
	// comments are preserved so that line-number accounting remains correct.
	if blockCommentExts[strings.ToLower(filepath.Ext(path))] {
		content = stripBlockComments(content)
	}

	var findings []AnalysisFinding
	var urls []ExtractedURL
	seen := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	// Minified bundles (esbuild/Vite IIFE output, vendor.js chunks) routinely
	// produce single lines >64 KB - the default Scanner buffer chokes with
	// "token too long". Bump the cap to 4 MB which covers any realistic
	// minified file we'd ship in a plugin (plugin-asset size limit is 50 MB
	// per file but a single line that big would already be pathological).
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// normalized is the line after obfuscation-defusing transformations.
		// It is used for pattern matching only - the raw line is preserved for
		// URL extraction and for the Pattern field in findings (so displayed
		// matches reflect normalized form but are still legible).
		normalized := normalizeLine(line)

		for _, rule := range analyzerRules {
			if isDoc {
				break // prose describes, it does not execute - URLs still noted below
			}
			if len(rule.extensions) > 0 {
				ext := strings.ToLower(filepath.Ext(path))
				if !slices.Contains(rule.extensions, ext) {
					continue
				}
			}

			if loc := rule.pattern.FindString(normalized); loc != "" {
				findings = append(findings, AnalysisFinding{
					Severity:    rule.severity,
					Category:    rule.category,
					File:        relPath,
					Line:        lineNum,
					Pattern:     loc,
					Description: rule.description,
				})
			}
		}

		// Scan for base64-encoded dangerous payloads.
		findings = append(findings, scanBase64Strings(normalized, relPath, lineNum)...)

		// Extract URLs
		for _, u := range urlLiteralRe.FindAllString(line, -1) {
			if !seen[u] {
				seen[u] = true
				urls = append(urls, ExtractedURL{URL: u, File: relPath, Line: lineNum, Type: "literal"})
			}
			if suspiciousSchemeRe.MatchString(u) {
				findings = append(findings, AnalysisFinding{
					Severity:    SeverityWarning,
					Category:    "suspicious_url",
					File:        relPath,
					Line:        lineNum,
					Pattern:     u,
					Description: "non-HTTP URL scheme",
				})
			}
		}
		if loc := urlTemplateRe.FindString(line); loc != "" && !seen[loc] {
			seen[loc] = true
			urls = append(urls, ExtractedURL{URL: loc, File: relPath, Line: lineNum, Type: "template"})
		}
		if urlConcatRe.MatchString(line) {
			key := relPath + ":" + fmt.Sprint(lineNum)
			if !seen[key] {
				seen[key] = true
				urls = append(urls, ExtractedURL{URL: "(concatenated URL)", File: relPath, Line: lineNum, Type: "concatenation"})
			}
		}
	}

	return findings, urls, scanner.Err()
}
