package pluginrunner

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Régression prod 2026-07-22 (weather-v2 en ERROR pendant 3 jours) : le plugin
// node avait été spawné pendant le scan de boot + le backfill keyframe
// (ffprobe en boucle) et avec nice 10, donc il a mis plus de 10 s à binder son
// socket. Le plafond de 10 s expirait AVANT le bind, sans aucune reprise, et le
// plugin était parqué en ERROR alors que son process tournait très bien.
func TestIPCConnectTimeoutLeavesRoomForASlowBoot(t *testing.T) {
	if ipcConnectTimeout < 60*time.Second {
		t.Fatalf("ipcConnectTimeout=%s : trop court pour un boot chargé (nice 10 + I/O du scan initial). "+
			"L'attente s'interrompt désormais dès la mort du process, donc un plafond large ne coûte rien.", ipcConnectTimeout)
	}
}

func TestWaitForSocketAbortsAsSoonAsTheProcessExits(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "plugin.sock")
	procDone := make(chan struct{})
	close(procDone) // le process est déjà mort

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	err := waitForSocket(ctx, sock, procDone, 0)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("attendu: une erreur quand le process est mort avant le bind")
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Errorf("l'erreur doit dire que le process est mort, pas parler de timeout : %v", err)
	}
	if elapsed > 2*time.Second {
		t.Errorf("attente de %s : la mort du process doit couper l'attente immédiatement", elapsed)
	}
}

func TestWaitForSocketKeepsPollingUntilABindLands(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "plugin.sock")
	procDone := make(chan struct{}) // process vivant

	go func() {
		time.Sleep(400 * time.Millisecond)
		l, err := net.Listen("unix", sock)
		if err != nil {
			return
		}
		t.Cleanup(func() { l.Close() })
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := waitForSocket(ctx, sock, procDone, 0); err != nil {
		t.Fatalf("un bind tardif mais réussi doit être vu : %v", err)
	}
}

// A plugin socket served by any process other than the plugin is refused,
// whoever got to the path first: the socket directories share a group, so a
// neighbour plugin could otherwise sit on it and receive this plugin's events
// and configuration. Here the listener is this test process.
func TestPluginSockRefusesAListenerThatIsNotThePlugin(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "plugin.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = waitForSocket(ctx, sock, make(chan struct{}), os.Getppid())
	if !errors.Is(err, errSockImpostor) {
		t.Fatalf("listener is not the plugin: %v, want errSockImpostor", err)
	}
	if conn, err := dialPluginSock(ctx, sock, os.Getppid()); err == nil {
		conn.Close()
		t.Fatal("the gRPC dialer must refuse the impostor too")
	}
	if err := waitForSocket(ctx, sock, make(chan struct{}), os.Getpid()); err != nil {
		t.Fatalf("the plugin's own listener must pass: %v", err)
	}
}
