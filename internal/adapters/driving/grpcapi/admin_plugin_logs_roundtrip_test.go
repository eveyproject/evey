package grpcapi

import (
	"context"
	"strconv"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driven/pluginrunner"
)

// Round-trip ÉCRITEUR → LECTEUR. Les tests existants de GetPluginLogs
// fabriquaient eux-mêmes un "<id>.log" en JSON, c'est-à-dire le format que le
// lecteur imaginait - personne n'avait vérifié contre ce que PluginLogger écrit
// réellement ("<id>-YYYY-MM-DD.log", lignes de stdout brut). Résultat : le
// visualiseur de logs plugin du panel admin renvoyait total=0 en permanence, et
// pendant l'incident weather-v2 du 2026-07-22 l'admin n'avait AUCUN détail au-delà
// du "timeout waiting for socket" générique - alors que le stdout du plugin
// contenait de quoi trancher.
func TestGetPluginLogsReadsWhatPluginLoggerActuallyWrites(t *testing.T) {
	logRoot := t.TempDir()

	// L'écriteur, tel qu'il tourne en prod : NewPluginLogger reçoit déjà le
	// sous-dossier "plugins" (cf wire_infra.go).
	logger := pluginrunner.NewPluginLogger(logRoot+"/plugins", 30)
	logger.WriteLog("plg-abc", "[INFO] plugin process started pid=4242")
	// Le stdout/stderr brut du plugin passe par Writer() : ni horodatage ni JSON.
	if _, err := logger.Writer("plg-abc").Write([]byte("weather-v2 listening on /tmp/evey/plugins/plg-abc/plugin.sock\n")); err != nil {
		t.Fatalf("write plugin stdout: %v", err)
	}
	logger.Close()

	d := newTestPluginHandler()
	d.logDir = logRoot

	resp, err := d.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "plg-abc"})
	if err != nil {
		t.Fatalf("GetPluginLogs: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("attendu 2 entrées lues depuis le fichier réellement écrit, obtenu %d (total=%d)", len(resp.Entries), resp.Total)
	}

	if resp.Entries[0].Level != "INFO" {
		t.Errorf("le niveau doit être extrait du préfixe [INFO], obtenu %q", resp.Entries[0].Level)
	}
	if resp.Entries[0].Message != "plugin process started pid=4242" {
		t.Errorf("message inattendu: %q", resp.Entries[0].Message)
	}
	if resp.Entries[0].Timestamp == nil {
		t.Error("l'horodatage RFC3339 en tête de ligne doit être parsé")
	}
	// Une ligne de stdout sans structure doit rester lisible telle quelle
	// plutôt que d'être silencieusement jetée.
	if resp.Entries[1].Message != "weather-v2 listening on /tmp/evey/plugins/plg-abc/plugin.sock" {
		t.Errorf("le stdout brut du plugin doit être conservé, obtenu %q", resp.Entries[1].Message)
	}
}

// Le pluginID n'est validé que contre les séparateurs et ".." : un motif de glob
// ne doit pas pour autant ratisser les logs des autres plugins.
func TestGetPluginLogsIgnoresGlobMetacharactersInPluginID(t *testing.T) {
	logRoot := t.TempDir()
	logger := pluginrunner.NewPluginLogger(logRoot+"/plugins", 30)
	logger.WriteLog("plg-aaa", "[INFO] secret du plugin A")
	logger.WriteLog("plg-bbb", "[INFO] secret du plugin B")
	logger.Close()

	d := newTestPluginHandler()
	d.logDir = logRoot

	resp, err := d.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "plg-*"})
	if err != nil {
		t.Fatalf("GetPluginLogs: %v", err)
	}
	if len(resp.Entries) != 0 {
		t.Errorf("un pluginID à métacaractères ne doit rien remonter, obtenu %d entrées", len(resp.Entries))
	}
}

// Le panel admin demande limit=100 SANS offset : il doit recevoir la fin du log,
// pas son début. Avec des fichiers de plusieurs jours concaténés, servir les 100
// premières lignes revient à montrer des logs antérieurs à la panne cherchée.
func TestGetPluginLogsWithoutOffsetServesTheTail(t *testing.T) {
	logRoot := t.TempDir()
	logger := pluginrunner.NewPluginLogger(logRoot+"/plugins", 30)
	for i := range 10 {
		logger.WriteLog("plg-tail", "[INFO] ligne "+strconv.Itoa(i))
	}
	logger.Close()

	d := newTestPluginHandler()
	d.logDir = logRoot

	resp, err := d.GetPluginLogs(context.Background(), &eveyv1.GetPluginLogsRequest{PluginId: "plg-tail", Limit: 3})
	if err != nil {
		t.Fatalf("GetPluginLogs: %v", err)
	}
	if resp.Total != 10 {
		t.Errorf("total doit rester le compte complet, obtenu %d", resp.Total)
	}
	if len(resp.Entries) != 3 {
		t.Fatalf("attendu 3 entrées, obtenu %d", len(resp.Entries))
	}
	if resp.Entries[0].Message != "ligne 7" || resp.Entries[2].Message != "ligne 9" {
		t.Errorf("attendu les lignes 7..9 (la fin), obtenu %q..%q",
			resp.Entries[0].Message, resp.Entries[2].Message)
	}
}
