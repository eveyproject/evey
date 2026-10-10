package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"strings"
)

// The top-level commands that need no config, database or network. Anything
// else boots the server.

//go:embed evey-completion.bash
var bashCompletion string

// runCLI handles `evey <command>` for the one-shot commands and reports
// whether it did; false means "boot the server".
func runCLI(args []string) (handled bool, err error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "version", "--version", "-v":
		b := buildID()
		fmt.Printf("evey %s\ncommit     %s\nbuilt      %s\n", b.Version, b.Commit, b.BuildDate)
		if v, code := tvRelease(); code > 0 {
			fmt.Printf("tv app     %s (versionCode %d)\n", v, code)
		}
		if v, code := mobileRelease(); code > 0 {
			fmt.Printf("mobile app %s (versionCode %d)\n", v, code)
		}
	case "help", "--help", "-h":
		printUsage(os.Stdout)
	case "admin":
		if err := runAdmin(args[1:]); err != nil {
			return true, fmt.Errorf("admin: %w", err)
		}
	case "completion":
		if len(args) < 2 || args[1] != "bash" {
			return true, fmt.Errorf("usage: evey completion bash")
		}
		fmt.Print(bashCompletion)
	case "man":
		switch {
		case len(args) == 1:
			renderRoff(os.Stdout, manual())
		case len(args) == 2 && args[1] == "--text":
			renderText(os.Stdout, manual())
		default:
			return true, fmt.Errorf("usage: evey man [--text]")
		}
	default:
		printUsage(os.Stderr)
		return true, fmt.Errorf("unknown command %q", args[0])
	}
	return true, nil
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `usage: evey                         start the server (config from EVEY_* env vars)
       evey admin <subcommand>     one-shot recovery / maintenance tools (evey admin --help)
       evey version                print the version, commit and build date
       evey completion bash        print the bash completion script
       evey man [--text]           print the manual page (roff, or plain text with --text)
       evey help                   this message
`)
}

// manEntry / manSection is the manual's shape; renderRoff and renderText are
// its two outputs, so the roff never drifts from the text `man evey` shows
// inside the container (which ships no man binary).
type manEntry struct {
	term string
	body []string
}

type manSection struct {
	title   string
	body    []string
	entries []manEntry
}

func manual() []manSection {
	admin := manSection{title: "ADMIN SUBCOMMANDS", body: []string{
		"Every subcommand runs instead of the server, reads the same EVEY_* environment as the server and exits when done. " +
			"The database must be reachable for most of them; none starts the server. `evey admin <subcommand> --help` prints the subcommand's flags.",
	}}
	for _, g := range adminGroups {
		admin.entries = append(admin.entries, manEntry{term: strings.ToUpper(g[:1]) + g[1:]})
		for _, c := range adminCommands {
			if c.group != g {
				continue
			}
			body := []string{c.short}
			if c.long != "" {
				body = append(body, strings.Split(c.long, "\n\n")...)
			}
			admin.entries = append(admin.entries, manEntry{term: "evey admin " + strings.TrimSpace(c.name+" "+c.args), body: body})
		}
	}
	return []manSection{
		{title: "NAME", body: []string{"evey - the Evey media server and its operator command line"}},
		{title: "SYNOPSIS", body: []string{
			"evey",
			"evey admin <subcommand> [flags]",
			"evey version | help | man [--text] | completion bash",
		}},
		{title: "DESCRIPTION", body: []string{
			"With no argument, evey boots the media server: it reads its configuration from EVEY_* environment variables, " +
				"connects to Postgres, applies pending migrations, and serves the gRPC API on EVEY_SERVER_PORT and the HTTP " +
				"gateway and web client on the next port.",
			"With a command, it runs that command and exits without starting the server. The admin subcommands are the " +
				"recovery and maintenance surface: what an operator needs when nobody can log in, the schema must move, " +
				"or the media library needs a repair.",
		}},
		{title: "COMMANDS", entries: []manEntry{
			{term: "version", body: []string{"Print the version, commit and build date. Needs no configuration: it answers from a container that will not start."}},
			{term: "admin <subcommand> [flags]", body: []string{"Run one operator subcommand; see ADMIN SUBCOMMANDS. Without a subcommand, print their list."}},
			{term: "completion bash", body: []string{"Print the bash completion script. Install it with: evey completion bash > /etc/bash_completion.d/evey"}},
			{term: "man [--text]", body: []string{"Print this manual as a roff page (install with: evey man > /usr/local/share/man/man1/evey.1), or as plain text with --text."}},
			{term: "help", body: []string{"Print the command summary."}},
		}},
		admin,
		{title: "ENVIRONMENT", body: []string{
			"Every command reads the server's configuration: EVEY_DB_HOST, EVEY_DB_PORT, EVEY_DB_USER, EVEY_DB_PASSWORD " +
				"(or EVEY_DB_PASSWORD_FILE), EVEY_DB_NAME, EVEY_DB_SSLMODE for the database; EVEY_BACKUP_DIR for the " +
				"pre-migration dumps; EVEY_JWT_PRIVATE_KEY_PATH for the signing key; EVEY_SERVER_PORT for load-test. " +
				"The full list is in docs/configuration.md.",
			"Inside the Docker image the entrypoint exports all of it, so `docker exec evey-server evey admin ...` needs nothing else.",
		}},
		{title: "EXIT STATUS", entries: []manEntry{
			{term: "0", body: []string{"Success. For load-test, admission_full (the box reached its ceiling) is a verdict, not a failure."}},
			{term: "1", body: []string{"The command failed, a doctor check failed, or the schema is dirty (migrate version)."}},
			{term: "86", body: []string{"The server drained after an admin asked for a restart; the container entrypoint relaunches it on this code and stops on any other."}},
		}},
		{title: "SEE ALSO", body: []string{
			"docs/runbooks/cli.md, docs/runbooks/recovery.md, docs/runbooks/migration-runbook.md, docs/configuration.md; mkvgo(1) for the media file toolbox shipped in the image.",
		}},
	}
}

func renderRoff(w io.Writer, sections []manSection) {
	fmt.Fprintf(w, ".TH EVEY 1 \"\" \"evey %s\" \"Evey Manual\"\n", buildID().Version)
	for _, s := range sections {
		fmt.Fprintf(w, ".SH %s\n", roffEscape(s.title))
		for _, p := range s.body {
			fmt.Fprintf(w, "%s\n.PP\n", roffEscape(p))
		}
		for _, e := range s.entries {
			if len(e.body) == 0 {
				fmt.Fprintf(w, ".SS %s\n", roffEscape(e.term))
				continue
			}
			fmt.Fprintf(w, ".TP\n\\fB%s\\fR\n", roffEscape(e.term))
			for i, p := range e.body {
				if i > 0 {
					fmt.Fprintln(w, ".IP")
				}
				fmt.Fprintln(w, roffEscape(p))
			}
		}
	}
}

func roffEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "-", `\-`)
	if strings.HasPrefix(s, ".") || strings.HasPrefix(s, "'") {
		s = `\&` + s
	}
	return s
}

func renderText(w io.Writer, sections []manSection) {
	fmt.Fprintf(w, "EVEY(1)                        evey %s\n\n", buildID().Version)
	for _, s := range sections {
		fmt.Fprintf(w, "%s\n", s.title)
		for _, p := range s.body {
			fmt.Fprintln(w, wrapText(p, 78, "    "))
			fmt.Fprintln(w)
		}
		for _, e := range s.entries {
			if len(e.body) == 0 {
				fmt.Fprintf(w, "  %s\n\n", e.term)
				continue
			}
			fmt.Fprintf(w, "    %s\n", e.term)
			for _, p := range e.body {
				fmt.Fprintln(w, wrapText(p, 78, "        "))
				fmt.Fprintln(w)
			}
		}
	}
}
