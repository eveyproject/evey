package music_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	appmusic "github.com/evey-project/evey/internal/app/music"
	"github.com/evey-project/evey/internal/domain/library"
	libfakes "github.com/evey-project/evey/internal/domain/library/fakes"
	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/music/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// fakeFetcher is a manual fake (no mock lib) for AlbumArtFetcher.
type fakeFetcher struct {
	data  []byte
	mime  string
	err   error
	calls atomic.Int32
}

func (f *fakeFetcher) Fetch(_ context.Context, _, _ string) ([]byte, string, error) {
	f.calls.Add(1)
	return f.data, f.mime, f.err
}

func newCmdWithFakes(t *testing.T) (*appmusic.CommandService, *fakes.FakeMusicTrackRepository, *fakes.FakeMusicScanner) {
	t.Helper()
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	cmd := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	return cmd, repo, scanner
}

func TestCommandService_FetchMissingAlbumArt_NilFetcher(t *testing.T) {
	cmd, _, _ := newCmdWithFakes(t)
	n, err := cmd.FetchMissingAlbumArt(context.Background(), "lib1")
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestCommandService_FetchMissingAlbumArt_EmptyDir(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	cmd := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	// Apply only fetcher via WithAlbumArtFetcher but dataDir="" → albumArtDir=="music_album_art" is non-empty...
	// To test the "albumArtDir == ''" branch we DON'T call WithAlbumArtFetcher.
	// Simulate by providing only fetcher via nil fetcher path already covered; so explicitly test:
	// WithAlbumArtFetcher with a non-empty dataDir sets both. The "empty dir" branch happens only when
	// fetcher is set but albumArtDir == "". Since WithAlbumArtFetcher always sets dir via filepath.Join,
	// the empty branch only trips when not configured at all → equivalent to nil fetcher case.
	// We still assert the no-op behavior for completeness.
	n, err := cmd.FetchMissingAlbumArt(context.Background(), "lib1")
	if err != nil || n != 0 {
		t.Fatalf("want (0,nil), got (%d,%v)", n, err)
	}
}

func TestCommandService_FetchMissingAlbumArt_HappyPath(t *testing.T) {
	cmd, repo, _ := newCmdWithFakes(t)
	dir := t.TempDir()
	fetcher := &fakeFetcher{data: []byte("IMGBYTES"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, dir)

	libID := "lib1"
	// Seed a track with album/artist but no cover art.
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID:          "t1",
		LibraryID:   shared.LibraryID(libID),
		Title:       "T",
		Artist:      "ArtistA",
		AlbumArtist: "ArtistA",
		Album:       "AlbumX",
		FilePath:    "/m/AlbumX/t1.mp3",
	})

	n, err := cmd.FetchMissingAlbumArt(context.Background(), libID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 fetched, got %d", n)
	}

	// Folder-identity: cover filename is hashed from the album folder path,
	// not the (artist|album) tuple, so two folders with identical tags keep
	// separate images.
	sum := sha256.Sum256([]byte("/m/AlbumX" + "|"))
	hash := hex.EncodeToString(sum[:])
	dst := filepath.Join(cmd.AlbumArtDir(), hash+".jpg")
	got, rerr := os.ReadFile(dst)
	if rerr != nil {
		t.Fatalf("expected file written: %v", rerr)
	}
	if string(got) != "IMGBYTES" {
		t.Fatalf("bad bytes written: %q", got)
	}

	// Verify track cover url was updated with /api/v1/music/album-art/<hash>.
	tr, _ := repo.GetByID(context.Background(), "t1")
	expected := "/api/v1/music/album-art/" + hash
	if tr == nil || tr.CoverArtURL != expected {
		t.Fatalf("cover url mismatch: got %+v, want %s", tr, expected)
	}
}

// Régression prod 2026-07-25 : le fetcher externe partait pour TOUS les albums
// dont l'image cache manquait sur disque, sans regarder si les fichiers audio
// portaient déjà leur pochette EMBARQUÉE - et écrasait ensuite le cover_art_url
// de toutes les pistes du dossier. Les MP3 de "Orchard Essentials" (Neon Harbor)
// embarquaient la bonne pochette et affichaient celle de Mixed Signals, devinée
// par iTunes. Le fichier audio est autoritaire : on ne va pas deviner à sa place.
func TestCommandService_FetchMissingAlbumArt_SkipsAlbumsThatAlreadyHaveEmbeddedArt(t *testing.T) {
	cmd, repo, _ := newCmdWithFakes(t)
	fetcher := &fakeFetcher{data: []byte("ITUNESGUESS"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, t.TempDir())

	libID := "lib1"
	embedded := "/api/v1/music/tracks/t1/art"
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "t1", LibraryID: shared.LibraryID(libID), Title: "At The Close",
		Artist: "Neon Harbor", AlbumArtist: "Neon Harbor", Album: "Orchard Essentials",
		FilePath: "/m/Essentials/01.mp3", CoverArtURL: embedded,
	})

	n, err := cmd.FetchMissingAlbumArt(context.Background(), libID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Fatalf("aucun fetch attendu pour un album à pochette embarquée, obtenu %d", n)
	}
	if got := fetcher.calls.Load(); got != 0 {
		t.Errorf("la source externe ne doit pas être interrogée (%d appels)", got)
	}
	tr, _ := repo.GetByID(context.Background(), "t1")
	if tr == nil || tr.CoverArtURL != embedded {
		t.Fatalf("la pochette embarquée doit être conservée intacte, obtenu %+v", tr)
	}
}

// mp3WithEmbeddedPicture forge un MP3 minimal portant une frame ID3v2.3 APIC,
// telle que dhowden/tag la lit. Pas de fixture binaire au dépôt : la structure
// du tag est justement ce que le test doit exercer.
func mp3WithEmbeddedPicture(t *testing.T) []byte {
	t.Helper()
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 'f', 'a', 'k', 'e'}

	var frame []byte
	frame = append(frame, "image/jpeg"...)
	frame = append(frame, 0x00) // fin du MIME
	frame = append(frame, 0x03) // type d'image : front cover
	frame = append(frame, 0x00) // description vide
	frame = append(frame, jpeg...)

	body := []byte{0x00} // encodage texte : ISO-8859-1
	body = append(body, frame...)

	syncsafe := func(n int) []byte {
		return []byte{byte(n >> 21 & 0x7F), byte(n >> 14 & 0x7F), byte(n >> 7 & 0x7F), byte(n & 0x7F)}
	}
	be32 := func(n int) []byte {
		return []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	}

	var frames []byte
	frames = append(frames, "APIC"...)
	frames = append(frames, be32(len(body))...) // ID3v2.3 : taille brute, pas syncsafe
	frames = append(frames, 0x00, 0x00)         // flags
	frames = append(frames, body...)

	out := []byte("ID3")
	out = append(out, 0x03, 0x00) // version 2.3.0
	out = append(out, 0x00)       // flags
	out = append(out, syncsafe(len(frames))...)
	out = append(out, frames...)
	return out
}

// Régression prod 2026-07-25, chemin de RÉPARATION. L'ancien fetch avait écrasé
// en base les URLs de pochette embarquée de tout le dossier ; comme un scan ne
// ré-upsert pas un fichier inchangé, elles ne revenaient jamais et l'album
// restait collé sur la mauvaise image. La base ne peut donc pas être crue sur
// l'absence de pochette : il faut rouvrir un fichier du dossier pour trancher.
func TestCommandService_FetchMissingAlbumArt_RestoresEmbeddedArtClobberedInDB(t *testing.T) {
	cmd, repo, scanner := newCmdWithFakes(t)
	fetcher := &fakeFetcher{data: []byte("ITUNESGUESS"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, t.TempDir())

	libID := "lib1"
	path := "/m/Essentials/01.mp3"
	scanner.FileData = map[string][]byte{path: mp3WithEmbeddedPicture(t)}

	// État exact de la prod : la piste PORTE une pochette dans son fichier, mais
	// sa ligne en base a été écrasée par une URL album-art.
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "t1", LibraryID: shared.LibraryID(libID), Title: "At The Close",
		Artist: "Neon Harbor", AlbumArtist: "Neon Harbor", Album: "Orchard Essentials",
		FilePath: path,
	})
	if err := repo.SetAlbumCoverArtURL(context.Background(), shared.LibraryID(libID),
		"/m/Essentials", "/api/v1/music/album-art/deadbeef"); err != nil {
		t.Fatalf("seed fetched art: %v", err)
	}

	n, err := cmd.FetchMissingAlbumArt(context.Background(), libID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 || fetcher.calls.Load() != 0 {
		t.Fatalf("la pochette locale doit court-circuiter la source externe (n=%d, appels=%d)", n, fetcher.calls.Load())
	}
	tr, _ := repo.GetByID(context.Background(), "t1")
	if tr == nil || tr.CoverArtURL != "/api/v1/music/tracks/t1/art" {
		t.Fatalf("la pochette embarquée doit être restaurée, obtenu %+v", tr)
	}
}

// Sans pochette dans le fichier, la récupération externe reste le seul recours :
// la réparation ci-dessus ne doit pas bloquer le cas normal.
func TestCommandService_FetchMissingAlbumArt_FallsBackToFetcherWithoutLocalArt(t *testing.T) {
	cmd, repo, scanner := newCmdWithFakes(t)
	fetcher := &fakeFetcher{data: []byte("IMGBYTES"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, t.TempDir())

	libID := "lib1"
	path := "/m/Crime/01.flac"
	scanner.FileData = map[string][]byte{path: []byte("pas un fichier taggé")}

	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "t9", LibraryID: shared.LibraryID(libID), Title: "Some Song",
		Artist: "Skyrider", AlbumArtist: "Skyrider", Album: "Song Of The Valley",
		FilePath: path,
	})

	n, err := cmd.FetchMissingAlbumArt(context.Background(), libID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 1 || fetcher.calls.Load() != 1 {
		t.Fatalf("la source externe doit être utilisée (n=%d, appels=%d)", n, fetcher.calls.Load())
	}
}

// Régression prod 2026-07-25, cas "From Nowhere" : aucune pochette dans les
// fichiers, et une image récupérée AVANT que le fetcher ne vérifie la
// correspondance - donc la pochette d'un autre album. Le garde-fou os.Stat
// scellait ce mauvais choix définitivement. Une image sans témoin doit être
// recontrôlée une fois, et retirée si la source ne reconnaît plus l'album.
func TestCommandService_FetchMissingAlbumArt_DropsAnUnverifiedImageTheSourceNoLongerMatches(t *testing.T) {
	cmd, repo, scanner := newCmdWithFakes(t)
	dir := t.TempDir()
	// os.ErrNotExist = "aucun résultat ne correspond à l'album demandé".
	fetcher := &fakeFetcher{err: os.ErrNotExist}
	cmd.WithAlbumArtFetcher(fetcher, dir)

	libID := "lib1"
	path := "/m/FromZero/01.flac"
	scanner.FileData = map[string][]byte{path: []byte("flac sans pochette")}
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "fz1", LibraryID: shared.LibraryID(libID), Title: "Some Track",
		Artist: "Neon Harbor", AlbumArtist: "Neon Harbor", Album: "From Nowhere",
		FilePath: path,
	})

	// Image héritée de l'ancien fetch non vérifié (la pochette de Mixed Signals),
	// SANS témoin de vérification.
	sum := sha256.Sum256([]byte("/m/FromZero" + "|"))
	hash := hex.EncodeToString(sum[:])
	stale := filepath.Join(cmd.AlbumArtDir(), hash+".jpg")
	if err := os.MkdirAll(cmd.AlbumArtDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("WRONGCOVER"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetAlbumCoverArtURL(context.Background(), shared.LibraryID(libID),
		"/m/FromZero", "/api/v1/music/album-art/"+hash); err != nil {
		t.Fatal(err)
	}

	if _, err := cmd.FetchMissingAlbumArt(context.Background(), libID); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fetcher.calls.Load() != 1 {
		t.Fatalf("l'image sans témoin doit être recontrôlée une fois, obtenu %d appels", fetcher.calls.Load())
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("l'image qui ne correspond à aucun album doit être retirée du cache")
	}
	tr, _ := repo.GetByID(context.Background(), "fz1")
	if tr == nil || tr.CoverArtURL != "" {
		t.Errorf("la pochette doit repasser à vide pour laisser le placeholder, obtenu %+v", tr)
	}
}

// Une image DÉJÀ vérifiée n'est jamais recontrôlée : le témoin évite de
// re-solliciter la source externe à chaque scan.
func TestCommandService_FetchMissingAlbumArt_TrustsAnAlreadyVerifiedImage(t *testing.T) {
	cmd, repo, _ := newCmdWithFakes(t)
	fetcher := &fakeFetcher{data: []byte("IMG"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, t.TempDir())

	libID := "lib1"
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "v1", LibraryID: shared.LibraryID(libID), Title: "Some Song",
		Artist: "Skyrider", AlbumArtist: "Skyrider", Album: "Song Of The Valley",
		FilePath: "/m/Crime/01.flac",
	})

	sum := sha256.Sum256([]byte("/m/Crime" + "|"))
	hash := hex.EncodeToString(sum[:])
	if err := os.MkdirAll(cmd.AlbumArtDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(cmd.AlbumArtDir(), hash+".jpg")
	if err := os.WriteFile(img, []byte("GOODCOVER"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(img+".matched", []byte("Skyrider|Song Of The Valley\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := cmd.FetchMissingAlbumArt(context.Background(), libID); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if fetcher.calls.Load() != 0 {
		t.Errorf("une image témoignée ne doit pas relancer de fetch (%d appels)", fetcher.calls.Load())
	}
	if b, _ := os.ReadFile(img); string(b) != "GOODCOVER" {
		t.Errorf("l'image vérifiée doit rester intacte, obtenu %q", b)
	}
}

// Un album dont SEULES quelques pistes portent une pochette embarquée compte
// comme pourvu : la pochette de l'album se lit sur l'une de ses pistes, donc
// aller en chercher une autre en externe ne peut que la contredire.
func TestCommandService_FetchMissingAlbumArt_PartialEmbeddedArtIsEnough(t *testing.T) {
	cmd, repo, _ := newCmdWithFakes(t)
	fetcher := &fakeFetcher{data: []byte("ITUNESGUESS"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, t.TempDir())

	libID := "lib1"
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "p1", LibraryID: shared.LibraryID(libID), Title: "A",
		Artist: "Band", AlbumArtist: "Band", Album: "Half Tagged",
		FilePath: "/m/Half/01.mp3", CoverArtURL: "/api/v1/music/tracks/p1/art",
	})
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "p2", LibraryID: shared.LibraryID(libID), Title: "B",
		Artist: "Band", AlbumArtist: "Band", Album: "Half Tagged",
		FilePath: "/m/Half/02.mp3",
	})

	n, err := cmd.FetchMissingAlbumArt(context.Background(), libID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 || fetcher.calls.Load() != 0 {
		t.Fatalf("aucun fetch attendu (n=%d, appels=%d)", n, fetcher.calls.Load())
	}
}

// Regression: an album whose cover_art_url is already set in the DB but whose
// cached image was wiped from disk (cache cleared) must be re-fetched, not left
// broken. The old "missing art = empty URL" gate skipped it forever.
func TestCommandService_FetchMissingAlbumArt_RefetchesWhenFileWiped(t *testing.T) {
	cmd, repo, _ := newCmdWithFakes(t)
	dir := t.TempDir()
	fetcher := &fakeFetcher{data: []byte("FRESHBYTES"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, dir)

	libID := "lib1"
	sum := sha256.Sum256([]byte("/m/AlbumX" + "|"))
	hash := hex.EncodeToString(sum[:])
	// Track already carries the cover URL, but the file on disk is GONE.
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID:          "t1",
		LibraryID:   shared.LibraryID(libID),
		Title:       "T",
		Artist:      "ArtistA",
		AlbumArtist: "ArtistA",
		Album:       "AlbumX",
		FilePath:    "/m/AlbumX/t1.mp3",
		CoverArtURL: "/api/v1/music/album-art/" + hash,
	})

	n, err := cmd.FetchMissingAlbumArt(context.Background(), libID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 refetched, got %d", n)
	}
	got, rerr := os.ReadFile(filepath.Join(cmd.AlbumArtDir(), hash+".jpg"))
	if rerr != nil {
		t.Fatalf("expected file re-created: %v", rerr)
	}
	if string(got) != "FRESHBYTES" {
		t.Fatalf("bad bytes: %q", got)
	}
}

// An album whose cached image already exists on disk must NOT trigger a fetch.
func TestCommandService_FetchMissingAlbumArt_SkipsWhenFilePresent(t *testing.T) {
	cmd, repo, _ := newCmdWithFakes(t)
	dir := t.TempDir()
	fetcher := &fakeFetcher{data: []byte("SHOULD-NOT-RUN"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, dir)

	libID := "lib1"
	sum := sha256.Sum256([]byte("/m/AlbumX" + "|"))
	hash := hex.EncodeToString(sum[:])
	// Pre-create the cached image so the gate skips the fetch. Le témoin
	// ".matched" fait désormais partie de « image présente » : une image SANS
	// témoin vient de l'ancien fetcher non vérifié et doit être recontrôlée une
	// fois (cf DropsAnUnverifiedImage…), donc son absence ne ferait plus sauter
	// le fetch.
	if err := os.MkdirAll(cmd.AlbumArtDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(cmd.AlbumArtDir(), hash+".jpg")
	if err := os.WriteFile(img, []byte("EXISTING"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(img+".matched", []byte("ArtistA|AlbumX\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID:          "t1",
		LibraryID:   shared.LibraryID(libID),
		Title:       "T",
		Artist:      "ArtistA",
		AlbumArtist: "ArtistA",
		Album:       "AlbumX",
		FilePath:    "/m/AlbumX/t1.mp3",
	})

	n, err := cmd.FetchMissingAlbumArt(context.Background(), libID)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 fetched (file present), got %d", n)
	}
	if fetcher.calls.Load() != 0 {
		t.Fatalf("fetcher must not be called when file present, called %d", fetcher.calls.Load())
	}
}

func TestCommandService_FetchMissingAlbumArt_NotExistErrorSkipped(t *testing.T) {
	cmd, repo, _ := newCmdWithFakes(t)
	dir := t.TempDir()
	fetcher := &fakeFetcher{err: os.ErrNotExist}
	cmd.WithAlbumArtFetcher(fetcher, dir)

	libID := "lib1"
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "t1", LibraryID: shared.LibraryID(libID),
		Artist: "A", AlbumArtist: "A", Album: "B", FilePath: "/m/t.mp3",
	})

	n, err := cmd.FetchMissingAlbumArt(context.Background(), libID)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0, got %d", n)
	}
	if fetcher.calls.Load() != 1 {
		t.Fatalf("expected fetcher call once, got %d", fetcher.calls.Load())
	}
	// No file written.
	entries, _ := os.ReadDir(cmd.AlbumArtDir())
	if len(entries) != 0 {
		t.Fatalf("expected empty dir, got %d entries", len(entries))
	}
}

func TestCommandService_FetchMissingAlbumArt_OtherErrorContinues(t *testing.T) {
	cmd, repo, _ := newCmdWithFakes(t)
	dir := t.TempDir()
	fetcher := &fakeFetcher{err: errors.New("network boom")}
	cmd.WithAlbumArtFetcher(fetcher, dir)

	libID := "lib1"
	// Two DIFFERENT folders so they count as two distinct albums under the
	// folder-identity grouping. Same folder would merge into one album.
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "t1", LibraryID: shared.LibraryID(libID),
		Artist: "A1", AlbumArtist: "A1", Album: "Alb1", FilePath: "/m/Alb1/1.mp3",
	})
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "t2", LibraryID: shared.LibraryID(libID),
		Artist: "A2", AlbumArtist: "A2", Album: "Alb2", FilePath: "/m/Alb2/2.mp3",
	})

	n, err := cmd.FetchMissingAlbumArt(context.Background(), libID)
	if err != nil {
		t.Fatalf("should not return err on fetch failures, got %v", err)
	}
	if n != 0 {
		t.Fatalf("want 0 fetched, got %d", n)
	}
	if fetcher.calls.Load() != 2 {
		t.Fatalf("expected 2 calls (loop continues), got %d", fetcher.calls.Load())
	}
}

func TestCommandService_FetchMissingAlbumArt_SkipsEmptyArtistOrAlbum(t *testing.T) {
	// ListAlbumsMissingArt in fake skips empty album/artist so fetcher should NOT be called.
	cmd, repo, _ := newCmdWithFakes(t)
	dir := t.TempDir()
	fetcher := &fakeFetcher{data: []byte("x"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, dir)

	libID := "lib1"
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "t1", LibraryID: shared.LibraryID(libID),
		Artist: "", AlbumArtist: "", Album: "", FilePath: "/m/1.mp3",
	})
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: "t2", LibraryID: shared.LibraryID(libID),
		Artist: "A", AlbumArtist: "A", Album: "", FilePath: "/m/2.mp3",
	})

	n, err := cmd.FetchMissingAlbumArt(context.Background(), libID)
	if err != nil || n != 0 {
		t.Fatalf("want (0,nil), got (%d,%v)", n, err)
	}
	if fetcher.calls.Load() != 0 {
		t.Fatalf("fetcher should not be called for empty artist/album, got %d", fetcher.calls.Load())
	}
}

func TestCommandService_FetchMissingAlbumArt_CtxCancelledMidLoop(t *testing.T) {
	cmd, repo, _ := newCmdWithFakes(t)
	dir := t.TempDir()
	fetcher := &fakeFetcher{data: []byte("x"), mime: "image/jpeg"}
	cmd.WithAlbumArtFetcher(fetcher, dir)

	libID := "lib1"
	for i := 0; i < 3; i++ {
		_, _ = repo.Upsert(context.Background(), music.Track{
			ID:        music.Track{}.ID + string(rune('a'+i)),
			LibraryID: shared.LibraryID(libID),
			Artist:    "A" + string(rune('a'+i)), AlbumArtist: "A" + string(rune('a'+i)),
			Album:    "B" + string(rune('a'+i)),
			FilePath: "/m/" + string(rune('a'+i)) + ".mp3",
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE call → first iteration returns ctx.Err()

	n, err := cmd.FetchMissingAlbumArt(ctx, libID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v (n=%d)", err, n)
	}
}

// ---------------- parseTrack (via ScanDirectory) ----------------

// buildMinimalMP3WithID3 returns a tiny buffer with an ID3v2 header containing
// a TIT2 (title) frame. Sufficient for tag.ReadFrom to parse without audio data.
func buildMinimalMP3WithID3(title string) []byte {
	// Frame: TIT2, size = len(title)+1 (encoding byte), flags 0, encoding 0x00 (ISO-8859-1)
	frameBody := append([]byte{0x00}, []byte(title)...)
	frameHeader := []byte{'T', 'I', 'T', '2'}
	size := len(frameBody)
	frameHeader = append(frameHeader,
		byte(size>>24), byte(size>>16), byte(size>>8), byte(size),
		0x00, 0x00,
	)
	frame := append(frameHeader, frameBody...)

	// ID3v2.3 header. Tag size is syncsafe 28-bit encoding of len(frame).
	tagSize := len(frame)
	b1 := byte((tagSize >> 21) & 0x7f)
	b2 := byte((tagSize >> 14) & 0x7f)
	b3 := byte((tagSize >> 7) & 0x7f)
	b4 := byte(tagSize & 0x7f)
	header := []byte{'I', 'D', '3', 0x03, 0x00, 0x00, b1, b2, b3, b4}
	out := append(header, frame...)
	// Append MPEG sync bytes so library doesn't choke looking past the tag (not strictly needed).
	return out
}

func TestCommandService_ParseTrack_NilProber(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	libID := "lib1"
	now := time.Now()
	path := "/m/song.mp3"
	data := buildMinimalMP3WithID3("MyTitle")
	scanner.Files = []music.ScannedAudioFile{{Path: path, Size: int64(len(data)), ModTime: now, MimeType: "audio/mpeg"}}
	scanner.FileData[path] = data
	scanner.Hashes[path] = "hash-1"

	cmd := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)
	// No prober configured.

	n, err := cmd.ScanDirectory(context.Background(), libID, "/m")
	if err != nil {
		t.Fatalf("scan err: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 track upserted, got %d", n)
	}
	got, _ := repo.ListAllByLibrary(context.Background(), shared.LibraryID(libID))
	if len(got) != 1 {
		t.Fatalf("want 1 track in repo, got %d", len(got))
	}
	tr := got[0]
	if tr.Duration != 0 || tr.Bitrate != 0 || tr.SampleRate != 0 || tr.Channels != 0 {
		t.Fatalf("prober-nil: expected zero audio fields, got %+v", tr)
	}
}

func TestCommandService_ParseTrack_ProberFillsFields(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	libID := "lib1"
	now := time.Now()
	path := "/m/song.mp3"
	data := buildMinimalMP3WithID3("MyTitle")
	scanner.Files = []music.ScannedAudioFile{{Path: path, Size: int64(len(data)), ModTime: now, MimeType: "audio/mpeg"}}
	scanner.FileData[path] = data
	scanner.Hashes[path] = "hash-1"

	prober := libfakes.NewFakeStreamProber()
	br := int64(320000)
	sr := 44100
	ch := 2
	probeDur, _ := shared.NewDurationMs(123456)
	prober.Results[path] = &library.ProbeResult{
		Duration: probeDur,
		Streams: []*library.MediaStream{
			// Video stream first to verify we skip non-audio.
			{StreamType: "video"},
			{StreamType: "audio", Bitrate: &br, SampleRate: &sr, Channels: &ch},
		},
	}

	cmd := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner).WithProber(prober)

	n, err := cmd.ScanDirectory(context.Background(), libID, "/m")
	if err != nil || n != 1 {
		t.Fatalf("scan failed: n=%d err=%v", n, err)
	}
	got, _ := repo.ListAllByLibrary(context.Background(), shared.LibraryID(libID))
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	tr := got[0]
	if tr.Duration < 123.4 || tr.Duration > 123.5 {
		t.Fatalf("duration mismatch: got %v", tr.Duration)
	}
	if tr.Bitrate != 320000 {
		t.Fatalf("bitrate: got %d", tr.Bitrate)
	}
	if tr.SampleRate != 44100 {
		t.Fatalf("samplerate: got %d", tr.SampleRate)
	}
	if tr.Channels != 2 {
		t.Fatalf("channels: got %d", tr.Channels)
	}
}

func TestCommandService_ParseTrack_ProberErrorIgnored(t *testing.T) {
	repo := fakes.NewFakeMusicTrackRepository()
	scanner := fakes.NewFakeMusicScanner()
	libID := "lib1"
	now := time.Now()
	path := "/m/song.mp3"
	data := buildMinimalMP3WithID3("MyTitle")
	scanner.Files = []music.ScannedAudioFile{{Path: path, Size: int64(len(data)), ModTime: now, MimeType: "audio/mpeg"}}
	scanner.FileData[path] = data
	scanner.Hashes[path] = "hash-1"

	prober := libfakes.NewFakeStreamProber()
	prober.Err = errors.New("probe fail")

	cmd := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), nil, scanner).WithProber(prober)

	n, err := cmd.ScanDirectory(context.Background(), libID, "/m")
	if err != nil || n != 1 {
		t.Fatalf("scan failed: n=%d err=%v", n, err)
	}
	got, _ := repo.ListAllByLibrary(context.Background(), shared.LibraryID(libID))
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	tr := got[0]
	if tr.Duration != 0 || tr.Bitrate != 0 {
		t.Fatalf("prober-err: expected zero audio fields, got dur=%v br=%d", tr.Duration, tr.Bitrate)
	}
	if tr.FilePath != path {
		t.Fatalf("filepath mismatch: %s", tr.FilePath)
	}
}

// fakeMusicSearchIndex records the tracks pushed to it, so the reconcile's
// rebuild can be asserted without a live Meilisearch.
type fakeMusicSearchIndex struct {
	indexed []music.Track
	err     error
}

func (f *fakeMusicSearchIndex) IndexTrack(_ context.Context, t music.Track) error {
	if f.err != nil {
		return f.err
	}
	f.indexed = append(f.indexed, t)
	return nil
}

func (f *fakeMusicSearchIndex) IndexTracksBatch(_ context.Context, ts []music.Track) error {
	if f.err != nil {
		return f.err
	}
	f.indexed = append(f.indexed, ts...)
	return nil
}

func (f *fakeMusicSearchIndex) RemoveTrack(_ context.Context, _ string) error { return nil }

func (f *fakeMusicSearchIndex) Search(_ context.Context, _ shared.LibraryID, _ string, _ int) ([]music.Track, error) {
	return nil, nil
}

// fakeMusicDocCounter stands in for the engine's DocumentCount so the reconcile
// decision (skip / in-sync / drift-rebuild) is unit-tested.
type fakeMusicDocCounter struct {
	count int64
	err   error
}

func (f fakeMusicDocCounter) DocumentCount(_ context.Context) (int64, error) { return f.count, f.err }

func newReconcileCmd(t *testing.T) (*appmusic.CommandService, *fakes.FakeMusicTrackRepository, *fakeMusicSearchIndex) {
	t.Helper()
	repo := fakes.NewFakeMusicTrackRepository()
	libRepo := libfakes.NewFakeLibraryRepository()
	if err := libRepo.Create(context.Background(), &library.Library{ID: shared.LibraryID("lib-m"), Type: library.Music, Name: "Music"}); err != nil {
		t.Fatalf("seed library: %v", err)
	}
	idx := &fakeMusicSearchIndex{}
	cmd := appmusic.NewCommandService(repo, sharedfakes.NewFakeEventBus(), libRepo, fakes.NewFakeMusicScanner()).WithSearchIndex(idx)
	return cmd, repo, idx
}

func seedTrack(repo *fakes.FakeMusicTrackRepository, id string) {
	_, _ = repo.Upsert(context.Background(), music.Track{
		ID: id, LibraryID: shared.LibraryID("lib-m"), Title: id, FilePath: "/m/" + id + ".mp3",
	})
}

func TestReconcileTracks_SkipsWhenEngineUnreachable(t *testing.T) {
	cmd, repo, idx := newReconcileCmd(t)
	seedTrack(repo, "t1")
	n, err := cmd.ReconcileTracks(context.Background(), idx, fakeMusicDocCounter{err: errors.New("connection refused")})
	if err != nil {
		t.Fatalf("an unreachable engine is a skip, not an error: %v", err)
	}
	if n != 0 || len(idx.indexed) != 0 {
		t.Fatalf("must not scan/index against a down engine, got n=%d indexed=%d", n, len(idx.indexed))
	}
}

func TestReconcileTracks_InSync_NoRebuild(t *testing.T) {
	cmd, repo, idx := newReconcileCmd(t)
	seedTrack(repo, "t1")
	seedTrack(repo, "t2")
	n, err := cmd.ReconcileTracks(context.Background(), idx, fakeMusicDocCounter{count: 2})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 || len(idx.indexed) != 0 {
		t.Fatalf("doc count matches db → no rebuild, got n=%d indexed=%d", n, len(idx.indexed))
	}
}

func TestReconcileTracks_DriftRebuilds(t *testing.T) {
	cmd, repo, idx := newReconcileCmd(t)
	seedTrack(repo, "t1")
	seedTrack(repo, "t2")
	seedTrack(repo, "t3")
	// Engine holds 1 doc, DB holds 3 - drift (writes dropped during an outage)
	// → full rebuild.
	n, err := cmd.ReconcileTracks(context.Background(), idx, fakeMusicDocCounter{count: 1})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 3 {
		t.Fatalf("want 3 reindexed on drift, got %d", n)
	}
	if len(idx.indexed) != 3 {
		t.Fatalf("want 3 tracks pushed to the engine, got %d", len(idx.indexed))
	}
}

// Stale docs (engine holds MORE than the DB) must NOT trigger a rebuild - a
// re-index only upserts and can't shrink the count, so it would loop every tick.
func TestReconcileTracks_StaleDocs_NoRebuild(t *testing.T) {
	cmd, repo, idx := newReconcileCmd(t)
	seedTrack(repo, "t1")
	seedTrack(repo, "t2")
	// Engine holds 5 docs, DB holds 2 → stale → skip (no loop).
	n, err := cmd.ReconcileTracks(context.Background(), idx, fakeMusicDocCounter{count: 5})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if n != 0 || len(idx.indexed) != 0 {
		t.Fatalf("stale-doc divergence must NOT rebuild (would loop), got n=%d indexed=%d", n, len(idx.indexed))
	}
}

// clearableMusicIndex is an engine that can be wiped, like Meilisearch.
type clearableMusicIndex struct {
	fakeMusicSearchIndex
	cleared int
}

func (c *clearableMusicIndex) Clear(_ context.Context) error { c.cleared++; return nil }

// An engine that can be wiped is wiped then rebuilt when it holds MORE docs
// than the DB (ghosts of a deleted library or a dropped delete): an upsert
// alone never shrinks it, and search kept serving the ghosts.
func TestReconcileTracks_StaleDocs_WipesThenRebuildsWhenTheEngineCan(t *testing.T) {
	cmd, repo, _ := newReconcileCmd(t)
	seedTrack(repo, "t1")
	seedTrack(repo, "t2")
	idx := &clearableMusicIndex{}
	n, err := cmd.ReconcileTracks(context.Background(), idx, fakeMusicDocCounter{count: 161})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if idx.cleared != 1 {
		t.Fatalf("want the index wiped once, got %d", idx.cleared)
	}
	if n != 2 || len(idx.indexed) != 2 {
		t.Fatalf("want the 2 DB tracks reindexed after the wipe, got n=%d indexed=%d", n, len(idx.indexed))
	}
}
