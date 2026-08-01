package ratelimit

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Le plafond de visiteurs (08-01) a fermé une fuite mémoire non bornée, mais en
// a ouvert une autre au même endroit : arrivé au plafond avec des entrées
// toutes FRAÎCHES - exactement le cas d'adresses qui tournent que le plafond
// vise - la passe de rétention ne supprime rien, on retire la plus froide, et
// la table revient au plafond. La requête SUIVANTE relance donc un parcours
// complet. Coût pour l'attaquant : un paquet. Coût pour le serveur : 50 000
// itérations en tenant l.mu, le mutex que prend AUSSI chaque appel légitime,
// connexion comprise. Le correctif de la fuite devenait un amplificateur de
// contention.
//
// Contrat épinglé : le coût d'une éviction ne doit pas dépendre de la taille de
// la table.

func fillToCap(t *testing.T, l *Limiter) {
	t.Helper()
	for i := 0; len(l.visitors) < maxVisitors; i++ {
		l.Allow(fmt.Sprintf("10.%d.%d.%d", i>>16&0xff, i>>8&0xff, i&0xff))
		if i > maxVisitors*4 {
			t.Fatalf("impossible de remplir la table (%d entrées)", len(l.visitors))
		}
	}
}

func TestEviction_CostDoesNotScaleWithTableSize(t *testing.T) {
	l := New(1000)
	defer l.Stop()

	fillToCap(t, l)

	l.mu.Lock()
	evictScanned = 0
	l.mu.Unlock()

	// Une adresse inédite : la table est au plafond, donc cet appel évince.
	l.Allow("203.0.113.7")

	l.mu.Lock()
	scanned, size := evictScanned, len(l.visitors)
	l.mu.Unlock()

	if scanned == 0 {
		t.Fatal("aucune éviction déclenchée : le test ne mesure rien")
	}
	if scanned > evictSample {
		t.Fatalf("une éviction a examiné %d entrées sur une table de %d (plafond permis : %d).\n"+
			"Le coût suit la taille de la table, donc chaque paquet d'un attaquant qui\n"+
			"change d'adresse fait payer un parcours complet SOUS le verrou que prend\n"+
			"aussi chaque connexion légitime.", scanned, size, evictSample)
	}
	if size > maxVisitors {
		t.Fatalf("la table a dépassé son plafond : %d > %d", size, maxVisitors)
	}
}

// Évincer un client À DÉCOUVERT lui rend son autorisation : sa requête suivante
// crée une entrée neuve avec un seau plein, ce qui annule exactement la
// limitation qu'on venait de lui appliquer.
//
// La garantie « un client actif n'est jamais évincé » ne tient que pour celui
// qui RÉESSAIE, puisque chaque tentative rafraîchit son lastCheck. Le client
// refusé UNE fois puis silencieux devient au contraire l'entrée la plus froide
// de la table dès qu'un attaquant la remplit d'adresses fraîches : il est alors
// le premier candidat à l'éviction. C'est précisément celui qu'il ne faut pas
// sortir.
//
// Sous un plafond dur, quelque chose doit partir ; le contrat n'est donc pas
// « jamais », mais « en dernier » : tant qu'il reste des entrées qui ne
// gagneraient RIEN à être évincées (seau non vide, elles passeraient de toute
// façon), ce sont elles qui partent.
//
// Attention au test qui se croit probant : viser UN client à découvert parmi
// 50 000 ne prouve rien, puisque l'échantillon de 64 ne le contient presque
// jamais - il survit par tirage, pas par politique. On mesure donc la politique
// sur une population : la moitié de la table à découvert, l'autre garnie, et on
// compte qui part.
func TestEviction_PrefersCallersThatOweNothing(t *testing.T) {
	l := NewWithBurst(5, 5)
	defer l.Stop()

	const evictions = 300
	now := time.Now()

	l.mu.Lock()
	// Toutes les entrées sont récentes (aucune périmée), donc la seule chose qui
	// peut départager un candidat de l'autre est l'état de son seau.
	for i := range maxVisitors {
		tokens := 3.0 // garni : l'évincer ne rend aucune autorisation
		if i%2 == 0 {
			tokens = 0 // à découvert : l'évincer annule sa limitation
		}
		l.visitors[fmt.Sprintf("k%d", i)] = &entry{
			tokens:    tokens,
			lastCheck: now.Add(-time.Duration(i%60) * time.Second),
		}
	}
	l.mu.Unlock()

	for i := 0; i < evictions; i++ {
		l.Allow(fmt.Sprintf("newcomer-%d", i))
	}

	l.mu.Lock()
	starvedLeft := 0
	for k, e := range l.visitors {
		if strings.HasPrefix(k, "k") && e.tokens < 1 {
			starvedLeft++
		}
	}
	l.mu.Unlock()

	const starvedAtStart = maxVisitors / 2
	starvedDropped := starvedAtStart - starvedLeft

	// Sans préférence, les victimes se répartissent moitié-moitié : ~150 des 300
	// évictions frapperaient un client à découvert. Avec, ce nombre doit être
	// proche de zéro. Le seuil laisse de la marge au tirage sans laisser passer
	// l'absence de politique.
	if starvedDropped > evictions/10 {
		t.Fatalf("%d des %d évictions ont frappé un client à découvert alors que la "+
			"table contenait autant d'entrées au seau garni.\nChacune de ces évictions "+
			"rend un seau plein à quelqu'un qu'on venait de refuser : c'est la "+
			"limitation elle-même qui est annulée, pas de la mémoire économisée.",
			starvedDropped, evictions)
	}
}
