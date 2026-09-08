# Operations

What to do when the server is already running: the day it misbehaves, the day
you move it, and the day you need it back.

The pages here assume a terminal and an administrator account. Almost everything
below is also reachable from the admin panel; the shell is the fallback for when
the panel is what stopped working.

| You need to | Read |
| --- | --- |
| Run a maintenance command on the server | [Command line](cli.md) |
| Find out why a file will not seek, or plays out of sync | [Media health](media-health.md) |
| Get a server that will not start back up | [Recovery](recovery.md) |
| Plan or undo a database migration | [Migrations](migration-runbook.md) |
| Work through a live incident | [Incidents](incident-runbook.md) |

## Before an incident, not during

Two things are worth checking while nothing is on fire, because both are
useless to discover under pressure: that [backups](../backups.md) are actually
being written where you think they are, and that you can restore one. A backup
nobody has restored is a hypothesis.
