---
source_sha: ac74ba14732aabf186129184b065fedaeb40d996e987374e385a8edc9d0e5c84
---
# Betrieb

Was zu tun ist, wenn der Server bereits läuft: an dem Tag, an dem er sich
danebenbenimmt, an dem Tag, an dem Sie ihn umziehen, und an dem Tag, an dem Sie
ihn zurückbrauchen.

Die Seiten hier setzen ein Terminal und ein Administratorkonto voraus. Fast
alles davon erreichen Sie auch über den Administrationsbereich; die Shell ist
der Rückfall für den Fall, dass genau dieser Bereich ausgefallen ist.

| Ihr Anliegen | Lesen Sie |
| --- | --- |
| Einen Wartungsbefehl auf dem Server ausführen | [Kommandozeile](cli.md) |
| Herausfinden, warum eine Datei nicht springt oder asynchron läuft | [Medienzustand](media-health.md) |
| Einen Server zurückholen, der nicht mehr startet | [Wiederherstellung](recovery.md) |
| Eine Datenbankmigration planen oder zurücknehmen | [Migrationen](migration-runbook.md) |
| Eine laufende Störung abarbeiten | [Störungen](incident-runbook.md) |

## Vor der Störung, nicht mittendrin

Zwei Dinge prüfen Sie besser, solange nichts brennt - unter Druck ist beides
nutzlos zu entdecken: dass [Sicherungen](../backups.md) tatsächlich dort
geschrieben werden, wo Sie sie vermuten, und dass Sie eine davon auch
zurückspielen können. Eine Sicherung, die nie jemand zurückgespielt hat, ist
eine Vermutung.
