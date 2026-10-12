package email

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	domainnotif "github.com/eveyproject/evey/internal/domain/notification"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
)

// ProfileGetter resolves a profile by ID.
type ProfileGetter interface {
	GetByID(ctx context.Context, id shared.ProfileID) (*domainuser.Profile, error)
}

// UserGetter resolves a user by ID.
type UserGetter interface {
	GetByID(ctx context.Context, id shared.UserID) (*domainuser.User, error)
}

// fanoutMemory bounds how long a (user, fan-out) pair is remembered: a fan-out
// publishes its per-profile events within seconds, an hour is ample.
const fanoutMemory = time.Hour

// NotificationEmailer subscribes to notification.created and sends emails,
// one per account per fan-out, in the recipient profile's language.
type NotificationEmailer struct {
	sender   Sender
	profiles ProfileGetter
	users    UserGetter
	// serverNameFn names the instance in the body (admin "Server name"); read
	// per mail so a rename applies without a restart. nil = "Evey".
	serverNameFn func() string
	translator   *eveyi18n.Translator
	localeFn     func() string // server language, for a profile without one
	baseURLFn    func() string // public URL linked from the body; "" = no link

	mu   sync.Mutex
	sent map[string]time.Time // userID|fanoutID -> first send
	now  func() time.Time
}

// NotificationEmailerOption configures optional NotificationEmailer inputs.
type NotificationEmailerOption func(*NotificationEmailer)

// WithNotificationLocale renders the mail through the push translations, in
// the profile's locale or else serverLocaleFn's.
func WithNotificationLocale(t *eveyi18n.Translator, serverLocaleFn func() string) NotificationEmailerOption {
	return func(e *NotificationEmailer) { e.translator, e.localeFn = t, serverLocaleFn }
}

// WithNotificationBaseURL links the server from the body.
func WithNotificationBaseURL(fn func() string) NotificationEmailerOption {
	return func(e *NotificationEmailer) { e.baseURLFn = fn }
}

func NewNotificationEmailer(sender Sender, profiles ProfileGetter, users UserGetter, serverNameFn func() string, opts ...NotificationEmailerOption) *NotificationEmailer {
	e := &NotificationEmailer{sender: sender, profiles: profiles, users: users, serverNameFn: serverNameFn, sent: map[string]time.Time{}, now: time.Now}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Register subscribes to notification.created on the given bus.
func (e *NotificationEmailer) Register(bus shared.EventBus) {
	bus.Subscribe("notification.created", e.handle)
}

func (e *NotificationEmailer) handle(ctx context.Context, ev shared.Event) {
	notif, ok := ev.(domainnotif.NotificationCreated)
	if !ok {
		return
	}
	// Skip the profile/user lookups while no SMTP host is in force.
	if c, ok := e.sender.(interface{ Configured() bool }); ok && !c.Configured() {
		return
	}

	profile, err := e.profiles.GetByID(ctx, notif.ProfileID)
	if err != nil {
		slog.Warn("email: get profile", slog.String("profile_id", string(notif.ProfileID)), slog.Any("err", err))
		return
	}

	u, err := e.users.GetByID(ctx, profile.UserID)
	if err != nil {
		slog.Warn("email: get user", slog.String("user_id", string(profile.UserID)), slog.Any("err", err))
		return
	}

	if u.Email == "" {
		return
	}

	// Profiles of one account share its address: the first profile of a
	// fan-out sends, the others are the same mail.
	key := string(u.ID) + "|" + notif.FanoutID
	if notif.FanoutID != "" && !e.claim(key) {
		return
	}

	locale := profile.Locale
	if locale == "" && e.localeFn != nil {
		locale = e.localeFn()
	}
	subject, body := e.render(notif, locale)
	if err := e.sender.Send([]string{u.Email}, subject, body); err != nil {
		slog.Warn("email: send notification", slog.String("to", u.Email), slog.Any("err", err))
		if notif.FanoutID != "" {
			e.release(key) // another profile of the account may retry
		}
	}
}

// claim records key and reports whether it was new; expired keys are pruned.
func (e *NotificationEmailer) claim(key string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := e.now()
	for k, at := range e.sent {
		if now.Sub(at) > fanoutMemory {
			delete(e.sent, k)
		}
	}
	if _, dup := e.sent[key]; dup {
		return false
	}
	e.sent[key] = now
	return true
}

func (e *NotificationEmailer) release(key string) {
	e.mu.Lock()
	delete(e.sent, key)
	e.mu.Unlock()
}

// render builds the subject (the push title) and a short plain-text body.
func (e *NotificationEmailer) render(n domainnotif.NotificationCreated, locale string) (subject, body string) {
	subject = e.tr(locale, n.TitleKey, n.TitleParams, n.TitleFallback)
	if subject == "" {
		// Publisher without a push title: never mail the "type:" storage prefix.
		subject = strings.TrimPrefix(n.Title, string(n.Type)+":")
	}
	server := serverNameOrDefault(e.serverNameFn)
	url := ""
	if e.baseURLFn != nil {
		url = strings.TrimRight(strings.TrimSpace(e.baseURLFn()), "/")
	}
	footer := e.tr(locale, "notifications.email.sent_by", map[string]string{"server": server}, "Sent by "+server+".")
	if url != "" {
		footer = e.tr(locale, "notifications.email.open", map[string]string{"server": server, "url": url}, "Open "+server+": "+url)
	}
	body = subject + "\n"
	if msg := strings.TrimSpace(n.Message); msg != "" {
		body += msg + "\n"
	}
	return subject, body + "\n" + footer + "\n"
}

// tr mirrors the push resolver: a missing key or translator yields fallback.
func (e *NotificationEmailer) tr(locale, key string, params map[string]string, fallback string) string {
	if key == "" || e.translator == nil {
		return fallback
	}
	if msg := e.translator.T(locale, key, params); msg != key {
		return msg
	}
	return fallback
}
