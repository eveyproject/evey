package sysinfo

import (
	"testing"
	"time"
)

// Le passage de sync.Once à un cache verrouillé (08-01) a corrigé un vrai
// blocage : une sonde qui pendait bloquait TOUS les appelants pour la vie du
// process. Mais « on ne mémorise que les succès » a introduit l'excès inverse :
// sur un hôte sans ffmpeg, chaque appel re-forkait un process en tenant
// ffmpegMu jusqu'à 3 s. Un panneau d'administration qui rafraîchit son état
// système paie alors 3 s par tour, sérialisés.
//
// Le contrat épinglé ici : un échec est mémorisé quelques secondes, puis
// réessayé. Les deux moitiés comptent - sans la mémoire on re-forke en boucle,
// sans le réessai un hôte réparé resterait marqué cassé jusqu'au redémarrage.

func resetFFmpegProbeState(t *testing.T) {
	t.Helper()
	ffmpegMu.Lock()
	ffmpegCached, ffmpegProbed, ffmpegProbes = "", false, 0
	ffmpegRetryAt = time.Time{}
	ffmpegMu.Unlock()
	t.Cleanup(func() {
		ffmpegMu.Lock()
		ffmpegCached, ffmpegProbed, ffmpegProbes = "", false, 0
		ffmpegRetryAt = time.Time{}
		ffmpegNow = time.Now
		ffmpegMu.Unlock()
	})
}

func probeCount() int {
	ffmpegMu.Lock()
	defer ffmpegMu.Unlock()
	return ffmpegProbes
}

func TestFFmpegVersion_FailedProbeIsNotRefiredOnEveryCall(t *testing.T) {
	resetFFmpegProbeState(t)

	// Un chemin qui ne peut pas exister : la sonde échoue à coup sûr.
	const missing = "/nonexistent/evey-test-ffmpeg"
	for i := 0; i < 5; i++ {
		if v := FFmpegVersion(missing); v != "" {
			t.Fatalf("une sonde en échec doit rendre une version vide, obtenu %q", v)
		}
	}

	if n := probeCount(); n != 1 {
		t.Fatalf("5 appels ont déclenché %d exécutions de ffmpeg au lieu d'une seule : "+
			"chaque appel re-forke un process en tenant le verrou global, donc un "+
			"panneau d'administration qui rafraîchit se sérialise", n)
	}
}

func TestFFmpegVersion_RetriesOnceTheWindowHasPassed(t *testing.T) {
	resetFFmpegProbeState(t)

	base := time.Now()
	ffmpegMu.Lock()
	ffmpegNow = func() time.Time { return base }
	ffmpegMu.Unlock()

	const missing = "/nonexistent/evey-test-ffmpeg"
	FFmpegVersion(missing)
	FFmpegVersion(missing)
	if n := probeCount(); n != 1 {
		t.Fatalf("dans la fenêtre, on attend 1 sonde, obtenu %d", n)
	}

	// L'hôte est « réparé » plus tard : la fenêtre doit rouvrir, sinon un
	// serveur qui tourne depuis des semaines resterait marqué sans ffmpeg.
	ffmpegMu.Lock()
	ffmpegNow = func() time.Time { return base.Add(ffmpegFailureRetryAfter + time.Second) }
	ffmpegMu.Unlock()

	FFmpegVersion(missing)
	if n := probeCount(); n != 2 {
		t.Fatalf("passé la fenêtre, la sonde doit être retentée (attendu 2, obtenu %d) : "+
			"sinon un échec transitoire condamne l'information jusqu'au redémarrage", n)
	}
}
