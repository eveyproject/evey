package user

import (
	"context"
	"strings"
	"testing"
)

// Le log d'échec de connexion est le SEUL endroit où une campagne de
// credential stuffing devient cherchable ligne par ligne : le compteur du
// limiteur ne retient qu'un agrégat par identifiant, il ne dit ni quand, ni
// depuis quelle vague, ni sur quels comptes. La mutation du 08-01 a montré que
// rien ne l'épinglait - les 11 mutants survivants d'auth.go tombaient tous dans
// des branches de log, donc on pouvait supprimer ces lignes sans qu'un seul
// test bronche.
//
// Le motif d'échec est porté par `reason` et il n'est pas décoratif :
//   - `unknown_user` en masse = une liste d'identifiants volée ailleurs qu'on
//     rejoue ici ; la bonne réponse est le blocage réseau.
//   - `bad_password` en masse sur peu de comptes = du brute force ciblé ; la
//     bonne réponse est de prévenir CES utilisateurs.
//
// Les confondre, c'est traiter les deux de la même façon, donc mal.

func TestLoginFailure_EmitsSearchableWarn(t *testing.T) {
	cases := []struct {
		name       string
		seed       bool
		username   string
		password   string
		wantReason string
	}{
		{
			name:       "identifiant inconnu",
			seed:       false,
			username:   "ghost",
			password:   "hunter2",
			wantReason: "reason=unknown_user",
		},
		{
			name:       "mot de passe faux sur un compte réel",
			seed:       true,
			username:   "alice",
			password:   "wrong-one",
			wantReason: "reason=bad_password",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Pas de t.Parallel : captureSlogBuffer remplace le logger global.
			buf, restore := captureSlogBuffer(t)
			defer restore()

			d := newAuthTestDeps()
			if tc.seed {
				seedAuthUser(t, d, "u1", tc.username, "correct-horse", false, true)
			}

			if _, _, err := d.svc.Login(context.Background(), tc.username, tc.password); err == nil {
				t.Fatal("cette connexion devait échouer")
			}

			out := buf.String()
			if !strings.Contains(out, "login failed") {
				t.Fatalf("aucun log 'login failed' émis - un échec de connexion doit rester\n"+
					"cherchable dans les logs, sinon une campagne est invisible.\nlogs:\n%s", out)
			}
			if !strings.Contains(out, "username="+tc.username) {
				t.Fatalf("le log ne nomme pas l'identifiant visé, on ne peut donc pas\n"+
					"distinguer un balayage large d'une attaque ciblée.\nlogs:\n%s", out)
			}
			if !strings.Contains(out, tc.wantReason) {
				t.Fatalf("motif attendu %q absent : sans lui, « compte inexistant » et\n"+
					"« mot de passe faux » se ressemblent, alors qu'ils appellent des\n"+
					"réponses différentes.\nlogs:\n%s", tc.wantReason, out)
			}
			// Une trace d'échec ne doit jamais transformer les logs en dictionnaire
			// de mots de passe : ce sont souvent de vrais mots de passe d'autres
			// services, et les logs sont lus par plus de monde que la base.
			if strings.Contains(out, tc.password) {
				t.Fatalf("le mot de passe soumis a fuité dans les logs\nlogs:\n%s", out)
			}
		})
	}
}
