package com.evey.dashsmoke;

import android.app.Activity;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.util.Log;
import android.view.SurfaceView;

import androidx.media3.common.C;
import androidx.media3.common.Format;
import androidx.media3.common.MediaItem;
import androidx.media3.common.MimeTypes;
import androidx.media3.common.PlaybackException;
import androidx.media3.common.Player;
import androidx.media3.common.TrackSelectionOverride;
import androidx.media3.common.Tracks;
import androidx.media3.exoplayer.ExoPlayer;

import java.util.ArrayList;
import java.util.List;

// Smoke test of an Evey manifest.mpd in Media3's ExoPlayer, the player the
// mobile and TV apps play through. Driven by adb (see smoke.sh): the url extra
// is the MPD, startMs the resume point, resumeOnly stops after the start.
// Every step logs "SMOKE PASS|FAIL <step> <detail>" under the EveySmoke tag,
// and "SMOKE DONE failed=<n>" at the end. The player is muted.
public class MainActivity extends Activity {
    private static final String TAG = "EveySmoke";
    private final Handler handler = new Handler(Looper.getMainLooper());
    private ExoPlayer player;
    private int failed;
    private final List<Runnable> steps = new ArrayList<>();

    @Override
    protected void onCreate(Bundle saved) {
        super.onCreate(saved);
        SurfaceView surface = new SurfaceView(this);
        setContentView(surface);
        String url = getIntent().getStringExtra("url");
        long startMs = getIntent().getLongExtra("startMs", 0);
        boolean resumeOnly = getIntent().getBooleanExtra("resumeOnly", false);

        player = new ExoPlayer.Builder(this).build();
        player.setVideoSurfaceView(surface);
        player.setVolume(0f);
        player.addListener(new Player.Listener() {
            @Override
            public void onPlayerError(PlaybackException error) {
                result(false, "error", error.getErrorCodeName() + " " + error.getMessage());
                done();
            }
        });
        player.setMediaItem(new MediaItem.Builder().setUri(url).setMimeType(MimeTypes.APPLICATION_MPD).build(), startMs);
        player.prepare();
        player.play();

        waitFor("start", 45000, () -> player.getCurrentPosition() >= startMs + 4000, () -> describe());
        if (!resumeOnly) {
            steps.add(() -> {
                player.seekTo(60000);
                waitFor("seek60", 45000, () -> player.getCurrentPosition() >= 63000, this::describe);
            });
            steps.add(() -> switchVideo(true));
            steps.add(() -> switchVideo(false));
            steps.add(this::switchAudio);
            steps.add(() -> {
                player.seekTo(Math.max(0, player.getDuration() - 6000));
                waitFor("eos", 60000, () -> player.getPlaybackState() == Player.STATE_ENDED, this::describe);
            });
        }
    }

    private void switchVideo(boolean lowest) {
        Tracks.Group group = groupOf(C.TRACK_TYPE_VIDEO, 0);
        if (group == null || group.length < 2) {
            result(true, "rung", "skipped: one rung");
            next();
            return;
        }
        int pick = 0;
        for (int i = 1; i < group.length; i++) {
            int h = group.getTrackFormat(i).height;
            if (lowest ? h < group.getTrackFormat(pick).height : h > group.getTrackFormat(pick).height) pick = i;
        }
        int want = group.getTrackFormat(pick).height;
        player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon()
                .setOverrideForType(new TrackSelectionOverride(group.getMediaTrackGroup(), pick)).build());
        long from = player.getCurrentPosition();
        waitFor("rung" + want + "p", 45000, () -> {
            Format f = player.getVideoFormat();
            return f != null && f.height == want && player.getCurrentPosition() >= from + 4000;
        }, this::describe);
    }

    private void switchAudio() {
        // The rendition not playing now (the player's own pick follows the
        // device language): the switch must change what is heard.
        Tracks.Group other = null;
        for (Tracks.Group g : player.getCurrentTracks().getGroups()) {
            if (g.getType() == C.TRACK_TYPE_AUDIO && !g.isSelected()) {
                other = g;
                break;
            }
        }
        if (other == null) {
            result(true, "audio", "skipped: one rendition");
            next();
            return;
        }
        Format want = other.getTrackFormat(0);
        player.setTrackSelectionParameters(player.getTrackSelectionParameters().buildUpon()
                .setOverrideForType(new TrackSelectionOverride(other.getMediaTrackGroup(), 0)).build());
        long from = player.getCurrentPosition();
        waitFor("audio_" + want.language, 45000, () -> {
            Format f = player.getAudioFormat();
            return f != null && want.language != null && want.language.equals(f.language) && player.getCurrentPosition() >= from + 4000;
        }, this::describe);
    }

    // The nth track group of a type in the current tracks, null when absent.
    private Tracks.Group groupOf(int type, int nth) {
        int n = 0;
        for (Tracks.Group g : player.getCurrentTracks().getGroups()) {
            if (g.getType() != type) continue;
            if (n++ == nth) return g;
        }
        return null;
    }

    private String describe() {
        Format v = player.getVideoFormat();
        Format a = player.getAudioFormat();
        return "pos=" + player.getCurrentPosition() + " dur=" + player.getDuration()
                + " video=" + (v == null ? "-" : v.height + "p " + v.codecs)
                + " audio=" + (a == null ? "-" : a.language + " " + a.codecs + " " + a.sampleRate + "Hz " + a.channelCount + "ch");
    }

    interface Cond { boolean ok(); }
    interface Detail { String get(); }

    private void waitFor(String step, long timeoutMs, Cond cond, Detail detail) {
        long end = System.currentTimeMillis() + timeoutMs;
        handler.post(new Runnable() {
            @Override
            public void run() {
                if (player.getPlayerError() != null) return;
                if (cond.ok()) {
                    result(true, step, detail.get());
                    next();
                } else if (System.currentTimeMillis() > end) {
                    result(false, step, "timeout " + detail.get());
                    next();
                } else {
                    handler.postDelayed(this, 250);
                }
            }
        });
    }

    private void next() {
        if (steps.isEmpty()) {
            done();
            return;
        }
        handler.post(steps.remove(0));
    }

    private void result(boolean ok, String step, String detail) {
        if (!ok) failed++;
        Log.i(TAG, "SMOKE " + (ok ? "PASS " : "FAIL ") + step + " " + detail);
    }

    private void done() {
        Log.i(TAG, "SMOKE DONE failed=" + failed);
        steps.clear();
        player.release();
        finish();
    }
}
