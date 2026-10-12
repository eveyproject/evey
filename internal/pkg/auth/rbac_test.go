package auth_test

import (
	"regexp"
	"strings"
	"testing"

	_ "github.com/eveyproject/evey/gen/evey/v1" // register proto service descriptors
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

var methodKeyRe = regexp.MustCompile(`^/evey\.v1\.\w+Service/\w+$`)

// registeredEveyMethods walks the compiled proto descriptors (registered by
// the blank gen/evey/v1 import above) and returns the full set of real
// evey.v1 RPC full names. Used to derive guards from ground truth instead of
// hand-typed lists that silently drift from the .proto files.
func registeredEveyMethods(t *testing.T) map[string]bool {
	t.Helper()
	real := make(map[string]bool)
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := range svcs.Len() {
			svc := svcs.Get(i)
			if !strings.HasPrefix(string(svc.FullName()), "evey.v1.") {
				continue // ignore google.* / grpc.* well-known/infra services
			}
			methods := svc.Methods()
			for j := range methods.Len() {
				real["/"+string(svc.FullName())+"/"+string(methods.Get(j).Name())] = true
			}
		}
		return true
	})
	require.NotEmpty(t, real, "no evey.v1 service methods discovered - proto descriptors not registered?")
	return real
}

// TestMethodTables_NoPhantomEntries: a key that names no real RPC can never
// match live traffic - it is dead weight that gives false assurance an
// endpoint is protected (it protects nothing, since the string never appears
// in a real request). Catches typos, renames, and removed RPCs across all
// three method-keyed tables in this package.
func TestMethodTables_NoPhantomEntries(t *testing.T) {
	real := registeredEveyMethods(t)

	for k := range auth.MethodPermissions {
		if !real[k] {
			t.Errorf("MethodPermissions[%q]: no such registered evey.v1 method (dead/typo'd entry)", k)
		}
	}
	for k := range auth.APIKeyScopeFor {
		if !real[k] {
			t.Errorf("APIKeyScopeFor[%q]: no such registered evey.v1 method (dead/typo'd entry)", k)
		}
	}
	for k := range auth.InternalMethods {
		if !real[k] {
			t.Errorf("InternalMethods[%q]: no such registered evey.v1 method (dead/typo'd entry)", k)
		}
	}
}

// TestAPIKeyTables_NoUnclassifiedRealMethods is the exhaustiveness guard for
// interceptors/auth.go's fail-closed default: every real evey.v1 RPC must
// resolve to at least one of PublicMethods, InternalMethods, RoleAdmin (in
// interceptors.DefaultRBACConfig().MethodRoles - covered for API keys by the
// admin deny-by-default that already required no scope/perm entry), or an
// entry in MethodPermissions/APIKeyScopeFor. Until 2026-08-01, 100 real RPCs
// matched none of these - a method absent from every table let an API key
// through regardless of its scopes, because the scope-check condition itself
// never triggered. Without this guard, the next RPC added to a .proto file
// reopens exactly that gap the moment someone forgets the entry; with it, the
// build fails instead.
func TestAPIKeyTables_NoUnclassifiedRealMethods(t *testing.T) {
	real := registeredEveyMethods(t)
	cfg := interceptors.DefaultRBACConfig()

	for m := range real {
		if cfg.PublicMethods[m] {
			continue
		}
		if auth.InternalMethods[m] {
			continue
		}
		if role, ok := cfg.MethodRoles[m]; ok && role == interceptors.RoleAdmin {
			continue
		}
		_, hasPerm := auth.MethodPermissions[m]
		_, hasScope := auth.APIKeyScopeFor[m]
		if hasPerm || hasScope {
			continue
		}
		t.Errorf("%s: not in PublicMethods, InternalMethods, RoleAdmin MethodRoles, "+
			"MethodPermissions, nor APIKeyScopeFor - an API key would reach it unconditionally "+
			"(interceptors/auth.go enforceRBAC step 2 fail-closed default has nothing to fail closed on)", m)
	}
}

// TestAdminMediaService_RealMethodsDenyByDefaultForAPIKeys proves the premise
// that lets InternalMethods skip AdminMediaService entirely: every real
// AdminMediaService RPC is RoleAdmin in interceptors.DefaultRBACConfig() and
// maps to neither an API-key scope nor a fine-grained permission, so
// interceptors/auth.go's step-2 deny-by-default ("isAdminMethod && !hasScope
// && !hasPerm", enforceRBAC) already blocks API keys from it - a redundant
// InternalMethods entry would add nothing. Read-only assertion against the
// interceptor's own config; does not modify interceptors/auth.go.
func TestAdminMediaService_RealMethodsDenyByDefaultForAPIKeys(t *testing.T) {
	cfg := interceptors.DefaultRBACConfig()

	var found int
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := range svcs.Len() {
			svc := svcs.Get(i)
			if string(svc.FullName()) != "evey.v1.AdminMediaService" {
				continue
			}
			methods := svc.Methods()
			for j := range methods.Len() {
				full := "/" + string(svc.FullName()) + "/" + string(methods.Get(j).Name())
				found++

				role, isClassified := cfg.MethodRoles[full]
				if !isClassified || role != interceptors.RoleAdmin {
					t.Errorf("%s: not RoleAdmin in interceptors.DefaultRBACConfig().MethodRoles - "+
						"deny-by-default would NOT cover it for API keys", full)
				}
				if _, hasScope := auth.APIKeyScopeFor[full]; hasScope {
					t.Errorf("%s: has an APIKeyScopeFor entry - deny-by-default no longer applies, "+
						"this RPC would be reachable by API key", full)
				}
				if _, hasPerm := auth.MethodPermissions[full]; hasPerm {
					t.Errorf("%s: has a MethodPermissions entry - deny-by-default no longer applies, "+
						"this RPC would be reachable by API key", full)
				}
			}
		}
		return true
	})
	require.Positive(t, found, "no evey.v1.AdminMediaService methods discovered - proto descriptors not registered?")
}

// TestMethodPermissions_AllKeysValidFormat catches typos in map keys.
func TestMethodPermissions_AllKeysValidFormat(t *testing.T) {
	for k := range auth.MethodPermissions {
		if !methodKeyRe.MatchString(k) {
			t.Errorf("MethodPermissions: malformed key %q (expected /evey.v1.XService/Method)", k)
		}
	}
}

// TestInternalMethods_AllKeysValidFormat catches typos in InternalMethods keys.
func TestInternalMethods_AllKeysValidFormat(t *testing.T) {
	for k := range auth.InternalMethods {
		if !methodKeyRe.MatchString(k) {
			t.Errorf("InternalMethods: malformed key %q", k)
		}
	}
}

// TestMethodPermissions_AllValuesNonEmpty: a permission value of "" would
// effectively disable the check for that method.
func TestMethodPermissions_AllValuesNonEmpty(t *testing.T) {
	for k, v := range auth.MethodPermissions {
		if strings.TrimSpace(v) == "" {
			t.Errorf("MethodPermissions[%q] = empty permission", k)
		}
	}
}

// TestMethodPermissions_KnownEndpoints: critical methods MUST have a permission.
// Removing or renaming the entry (= RBAC bypass) fails this test.
func TestMethodPermissions_KnownEndpoints(t *testing.T) {
	expected := map[string]string{
		"/evey.v1.LibraryService/CreateLibrary":       auth.PermLibraryCreate,
		"/evey.v1.LibraryService/DeleteLibrary":       auth.PermLibraryDelete,
		"/evey.v1.LibraryService/ScanLibrary":         auth.PermLibraryScan,
		"/evey.v1.LibraryService/GetLibrary":          auth.PermLibraryRead,
		"/evey.v1.LibraryService/ListLibraries":       auth.PermLibraryRead,
		"/evey.v1.LibraryService/GetMediaItem":        auth.PermMediaRead,
		"/evey.v1.LibraryService/SearchMedia":         auth.PermMediaRead,
		"/evey.v1.UserService/CreateUser":             auth.PermUserCreate,
		"/evey.v1.UserService/DeleteUser":             auth.PermUserDelete,
		"/evey.v1.UserService/DisableUser":            auth.PermUserDisable,
		"/evey.v1.UserService/UpdateUser":             auth.PermUserUpdate,
		"/evey.v1.UserService/ListUsers":              auth.PermUserRead,
		"/evey.v1.PlaybackService/StartStream":        auth.PermPlaybackStart,
		"/evey.v1.PlaybackService/StopStream":         auth.PermPlaybackStop,
		"/evey.v1.TranscodingService/StartTranscode":  auth.PermTranscodeStart,
		"/evey.v1.TranscodingService/StopTranscode":   auth.PermTranscodeStop,
		"/evey.v1.AdminPluginService/InstallPlugin":   auth.PermPluginInstall,
		"/evey.v1.AdminPluginService/UninstallPlugin": auth.PermPluginManage,
	}
	for method, perm := range expected {
		got, ok := auth.MethodPermissions[method]
		if !ok {
			t.Errorf("MethodPermissions missing critical method %q (RBAC gap)", method)
			continue
		}
		if got != perm {
			t.Errorf("MethodPermissions[%q] = %q, want %q", method, got, perm)
		}
	}
}

// TestInternalMethods_BlockListForAPIKeys: methods that MUST NOT be callable
// via API key. Removal = privilege escalation via API key.
func TestInternalMethods_BlockListForAPIKeys(t *testing.T) {
	mustBeInternal := []string{
		// Auth/session
		"/evey.v1.UserService/Login",
		"/evey.v1.UserService/RefreshToken",
		"/evey.v1.UserService/ResetPassword",
		"/evey.v1.UserService/RegisterWithInvite",
		"/evey.v1.UserService/InitiateQuickConnect",
		"/evey.v1.UserService/CheckQuickConnect",
		// The step that GRANTS the session. An API key able to call it would
		// escalate itself into a full JWT session on a device it controls.
		"/evey.v1.UserService/AuthorizeQuickConnect",
		// API key management itself
		"/evey.v1.UserService/CreateAPIKey",
		"/evey.v1.UserService/ListAPIKeys",
		"/evey.v1.UserService/RevokeAPIKey",
		// User lifecycle
		"/evey.v1.UserService/CreateUser",
		"/evey.v1.UserService/DeleteUser",
		"/evey.v1.UserService/DisableUser",
		// System / config
		"/evey.v1.AdminSystemService/SetupWizard",
		"/evey.v1.AdminSystemService/UpdateServerConfig",
		"/evey.v1.AdminSystemService/GetServerConfig",
		"/evey.v1.ServerConfigService/SetServerConfigEntry",
		"/evey.v1.ServerConfigService/DeleteServerConfigEntry",
		// Backups
		"/evey.v1.BackupService/CreateBackup",
		"/evey.v1.BackupService/DeleteBackup",
		// Library access
		"/evey.v1.LibraryAccessService/SetUserLibraryAccess",
		"/evey.v1.LibraryAccessService/RevokeAllUserLibraryAccess",
		// Plugin admin
		"/evey.v1.AdminPluginService/InstallPlugin",
		"/evey.v1.AdminPluginService/UninstallPlugin",
		"/evey.v1.AdminPluginService/EnablePlugin",
		"/evey.v1.AdminPluginService/DisablePlugin",
	}
	for _, m := range mustBeInternal {
		if !auth.InternalMethods[m] {
			t.Errorf("InternalMethods missing %q - API key would be allowed (privilege escalation risk)", m)
		}
	}
}

// TestInternalMethods_AllValuesTrue: a `false` entry would silently allow API keys.
func TestInternalMethods_AllValuesTrue(t *testing.T) {
	for k, v := range auth.InternalMethods {
		if !v {
			t.Errorf("InternalMethods[%q] = false (entry must be true to block API keys)", k)
		}
	}
}

// TestPermissions_SemanticConsistency: read-style methods should map to *.read
// (or other read-ish) perms; write-style methods should not map to *.read.
func TestPermissions_SemanticConsistency(t *testing.T) {
	readPrefixes := []string{"Get", "List", "Search", "Filter"}
	writePrefixes := []string{"Create", "Update", "Delete", "Set", "Remove", "Disable", "Refresh"}

	readPerms := map[string]bool{
		auth.PermLibraryRead:  true,
		auth.PermMediaRead:    true,
		auth.PermMetadataRead: true,
		auth.PermActivityRead: true,
		auth.PermAuditRead:    true,
		auth.PermUserRead:     true,
	}
	writePerms := map[string]bool{
		auth.PermLibraryCreate:   true,
		auth.PermLibraryUpdate:   true,
		auth.PermLibraryDelete:   true,
		auth.PermMediaUpdate:     true,
		auth.PermMediaDelete:     true,
		auth.PermUserCreate:      true,
		auth.PermUserUpdate:      true,
		auth.PermUserDelete:      true,
		auth.PermUserDisable:     true,
		auth.PermProfileCreate:   true,
		auth.PermProfileUpdate:   true,
		auth.PermProfileDelete:   true,
		auth.PermMetadataUpdate:  true,
		auth.PermMetadataRefresh: true,
		auth.PermSubtitleWrite:   true,
		auth.PermPluginInstall:   true,
		auth.PermPluginManage:    true,
	}

	for method, perm := range auth.MethodPermissions {
		// extract method name after last '/'
		idx := strings.LastIndex(method, "/")
		if idx < 0 {
			continue
		}
		name := method[idx+1:]

		isRead := false
		for _, p := range readPrefixes {
			if strings.HasPrefix(name, p) {
				isRead = true
				break
			}
		}
		isWrite := false
		for _, p := range writePrefixes {
			if strings.HasPrefix(name, p) {
				isWrite = true
				break
			}
		}

		// Whitelist: methods whose prefix doesn't reflect read/write semantics.
		switch name {
		case "ScanLibrary", "StartStream", "StopStream", "ReportProgress",
			"MarkWatched", "MarkUnwatched", "StartTranscode", "StopTranscode":
			continue
		}

		if isRead && writePerms[perm] {
			t.Errorf("read-like method %q assigned write permission %q", method, perm)
		}
		if isWrite && readPerms[perm] {
			t.Errorf("write-like method %q assigned read permission %q", method, perm)
		}
	}
}

// TestAllGRPCServicesCovered: the curated list of critical services must be
// reachable through at least one of the layers documented on InternalMethods:
// an entry in this package's own tables (MethodPermissions/InternalMethods),
// OR RoleAdmin in interceptors.DefaultRBACConfig().MethodRoles - the
// deny-by-default second layer. AdminMediaService is why the second layer
// counts here: its real RPCs were only ever "covered" by three InternalMethods
// entries naming nonexistent methods (removed; see
// TestAdminMediaService_RealMethodsDenyByDefaultForAPIKeys for the proof they
// were redundant), so requiring layer one alone would make this test blind to
// the very case it exists to catch.
func TestAllGRPCServicesCovered(t *testing.T) {
	criticalServices := []string{
		"LibraryService",
		"UserService",
		"PlaybackService",
		"TranscodingService",
		"MetadataService",
		"ActivityService",
		"AdminPluginService",
		"AdminSystemService",
		"AdminTaskService",
		"AdminSessionService",
		"AdminMediaService",
		"BackupService",
		"ServerConfigService",
		"RemoteAccessService",
		"LibraryAccessService",
	}
	cfg := interceptors.DefaultRBACConfig()
	covered := map[string]bool{}
	for k := range auth.MethodPermissions {
		for _, s := range criticalServices {
			if strings.Contains(k, "/evey.v1."+s+"/") {
				covered[s] = true
			}
		}
	}
	for k := range auth.InternalMethods {
		for _, s := range criticalServices {
			if strings.Contains(k, "/evey.v1."+s+"/") {
				covered[s] = true
			}
		}
	}
	for k, role := range cfg.MethodRoles {
		if role != interceptors.RoleAdmin {
			continue
		}
		for _, s := range criticalServices {
			if strings.Contains(k, "/evey.v1."+s+"/") {
				covered[s] = true
			}
		}
	}
	for _, s := range criticalServices {
		if !covered[s] {
			t.Errorf("service %q has zero entries in MethodPermissions, InternalMethods, or "+
				"interceptors RoleAdmin (RBAC blind spot)", s)
		}
	}
}

// TestMethodPermissions_NoDuplicateCaseInsensitive catches accidental
// near-duplicate keys (e.g. differing only in case) which would mean one entry
// silently overrides the other.
func TestMethodPermissions_NoDuplicateCaseInsensitive(t *testing.T) {
	seen := map[string]string{}
	for k := range auth.MethodPermissions {
		lk := strings.ToLower(k)
		if prev, ok := seen[lk]; ok && prev != k {
			t.Errorf("MethodPermissions case-duplicate keys: %q vs %q", prev, k)
		}
		seen[lk] = k
	}
}
