package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	appdemo "github.com/evey-project/evey/internal/app/demo"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/auth"
	"github.com/evey-project/evey/internal/pkg/demo"
	"github.com/evey-project/evey/internal/pkg/eventbus"
)

// demo-seed -- run the demo bootstrap by hand, without the server: the `demo`
// administrator, the visitor accounts and their profiles, the four libraries,
// and with --media the freely licensed files as well. The server does all of
// this itself at every boot with EVEY_DEMO=1; the command exists to seed an
// instance ahead of its first boot (before the host snapshot the restore
// script replays) and to inspect what the bootstrap would do.
//
// Refuses to run unless EVEY_DEMO=1: this is seed data for the public demo,
// not something to drop into a real server by mistake. Idempotent.

// repoLibraries stands in for the library service, which needs the scanner
// and the probers a one-shot command has no reason to build: libraries are
// written through the repository and scanned by the server at its next boot
// (scan_on_startup is set on them).
type repoLibraries struct{ repo *postgres.LibraryRepository }

func (l repoLibraries) CreateLibrary(ctx context.Context, lib *library.Library) error {
	return l.repo.Create(ctx, lib)
}

func (l repoLibraries) UpdateLibrary(ctx context.Context, lib *library.Library) error {
	return l.repo.Update(ctx, lib)
}

func (l repoLibraries) ScanLibrary(context.Context, string) (int, int, error) { return 0, 0, nil }

func (l repoLibraries) DeleteLibrary(ctx context.Context, id string) error {
	return l.repo.Delete(ctx, shared.LibraryID(id))
}

func adminDemoSeed(args []string) error {
	fs := adminFlags("demo-seed")
	apply := fs.Bool("apply", false, "write (default: print what would happen)")
	media := fs.Bool("media", false, "also download the manifest's media into EVEY_DEMO_MEDIA_DIR (gigabytes)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, db, cleanup, err := openAdminDB()
	if err != nil {
		return err
	}
	defer cleanup()
	if !cfg.Server.Demo {
		return errors.New("demo-seed: EVEY_DEMO is not set - this seeds the public demo, refusing on any other instance")
	}
	if !*apply {
		fmt.Printf("would ensure the %s / %s administrator, the visitor accounts (famille, solo, invite, ancien),\n", demo.Username, demo.Password)
		fmt.Printf("the %d demo libraries under %s", len(demo.Libraries), cfg.Server.DemoMediaDir)
		if *media {
			fmt.Printf(" and download the %d manifest files", len(demo.Manifest))
		}
		fmt.Println(".\n(dry run - add --apply to write)")
		return nil
	}

	timeout := 30 * time.Minute
	if *media {
		timeout = 6 * time.Hour // gigabytes from public mirrors
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	users := postgres.NewUserRepository(db)
	profiles := postgres.NewProfileRepository(db)
	hasher := auth.BcryptHasher{}
	bus := eventbus.New()
	uow := postgres.NewTxUnit(db)
	setup := appuser.NewCommandService(users, profiles, postgres.NewRefreshTokenRepository(db),
		postgres.NewRoleRepository(db), postgres.NewUserRoleRepository(db), hasher, nil, bus,
		7*24*time.Hour, postgres.NewQuickConnectRepository(db),
		appuser.WithUnitOfWork(uow), appuser.WithMinPasswordLength(demo.MinPasswordLength))
	accounts := appuser.NewUserService(users, profiles, postgres.NewRefreshTokenRepository(db), hasher, bus, uow,
		appuser.UserWithMinPasswordLength(demo.MinPasswordLength))
	libRepo := postgres.NewLibraryRepository(db)
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	boot := appdemo.NewBootstrapper(users, profiles, setup, accounts, libRepo, repoLibraries{libRepo}, hasher, cfg.Server.DemoMediaDir, log).
		WithLibraryAccess(postgres.NewLibraryAccessRepository(db)).
		WithMarker(postgres.NewServerConfigRepository(db)).
		WithAdminAccountIf(cfg.Server.DemoExtended, cfg.Server.DemoUser, cfg.Server.DemoPassword)

	if aerr := boot.EnsureAdmin(ctx); aerr != nil {
		return aerr
	}
	if serr := boot.SeedAccounts(ctx); serr != nil {
		return serr
	}
	libs, _, lerr := boot.EnsureLibraries(ctx)
	if lerr != nil {
		return lerr
	}
	if cerr := boot.ConfineAccounts(ctx, libs); cerr != nil {
		return cerr
	}
	if merr := boot.MarkBootstrapped(ctx); merr != nil {
		return merr
	}
	if *media {
		return boot.DownloadMedia(ctx, libs)
	}
	fmt.Println("done - the media is downloaded by the server at boot (or now with --media)")
	return nil
}
