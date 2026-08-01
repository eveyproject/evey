package transcoding

import (
	"testing"
	"time"

	"github.com/evey-project/evey/internal/pkg/bgloop"
)

// bgloop.Tick (08-01) a rattrapé les panics des boucles de fond pour qu'un
// mauvais tick ne tue plus le serveur. Mais un `recover` ne relâche QUE les
// `defer` : une section critique qui déverrouille par un appel ordinaire en fin
// de bloc voit son mutex rester pris pour toujours.
//
// C'est une régression, pas un reste : AVANT le filet, le panic tuait le
// process et Docker relançait en quelques secondes. APRÈS, le process survit
// avec `p.mu` tenu - or c'est le MÊME verrou que prend EnsureSegmentRange à
// chaque demande de segment. Donc toute lecture par le chemin logiciel se fige,
// `/healthz` répond toujours 200 (il ne touche pas le producteur), et aucun
// superviseur ne redémarre quoi que ce soit. Le filet transforme un crash
// visible et auto-réparé en panne silencieuse et définitive.
//
// Ces tests épinglent la seule chose qui compte : après un tick qui panique,
// le verrou doit être REPRENABLE.

// panicSessions renvoie une table de sessions qui fait paniquer n'importe quel
// balayage : l'entrée nulle déréférence dès le premier `s.mu.Lock()`.
func panicSessions() map[string][]*swSession {
	return map[string][]*swSession{"boom": {nil}}
}

func TestSweepIdle_ReleasesProducerLockOnPanic(t *testing.T) {
	p := &SoftwareProducer{
		sessions: panicSessions(),
		opts:     SoftwareProducerOptions{InactivityTimeout: time.Minute},
	}

	bgloop.Tick("test.sweepIdle", p.sweepIdle)

	if !p.mu.TryLock() {
		t.Fatal("p.mu est resté verrouillé après un tick qui a paniqué : toute demande " +
			"de segment se bloquerait pour toujours, sans crash ni redémarrage")
	}
	p.mu.Unlock()
}

func TestSweepKillPaused_ReleasesProducerLockOnPanic(t *testing.T) {
	p := &SoftwareProducer{
		sessions: panicSessions(),
		opts:     SoftwareProducerOptions{KillPausedAfter: time.Minute},
	}

	bgloop.Tick("test.sweepKillPaused", p.sweepKillPaused)

	if !p.mu.TryLock() {
		t.Fatal("p.mu est resté verrouillé après un tick qui a paniqué")
	}
	p.mu.Unlock()
}

// Pas de test pour sweepPause : il relâche p.mu AVANT la boucle qui peut
// paniquer, donc un test « le verrou est repris » y passerait sans rien
// prouver. Sa section critique par session (s.mu autour du SIGSTOP) est
// néanmoins passée en defer pour la même raison.

func TestOrchestratorSweepIdle_ReleasesLockOnPanic(t *testing.T) {
	o := &SegmentOrchestrator{
		sessions: map[string][]*orchestratorSession{"boom": {nil}},
	}

	bgloop.Tick("test.orchestrator.sweepIdle", o.sweepIdle)

	if !o.mu.TryLock() {
		t.Fatal("o.mu est resté verrouillé après un tick qui a paniqué : les sessions " +
			"matérielles ne pourraient plus ni démarrer ni s'arrêter")
	}
	o.mu.Unlock()
}
