package auth_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/pkg/auth"
)

var methodKeyRe = regexp.MustCompile(`^/evey\.v1\.\w+Service/\w+$`)

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
		"/evey.v1.ActivityService/GetAuditLog":        auth.PermAuditRead,
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
		"/evey.v1.UserService/ChangePassword",
		"/evey.v1.UserService/ForgotPassword",
		"/evey.v1.UserService/ResetPassword",
		"/evey.v1.UserService/VerifyEmail",
		"/evey.v1.UserService/RegisterWithInvite",
		"/evey.v1.UserService/InitiateQuickConnect",
		"/evey.v1.UserService/CheckQuickConnect",
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

// TestAllGRPCServicesCovered: each service referenced in MethodPermissions has
// at minimum one mapped method, AND the curated list of critical services is
// represented in either MethodPermissions or InternalMethods.
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
	for _, s := range criticalServices {
		if !covered[s] {
			t.Errorf("service %q has zero entries in MethodPermissions or InternalMethods (RBAC blind spot)", s)
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
